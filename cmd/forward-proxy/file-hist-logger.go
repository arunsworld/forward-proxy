package main

import (
	"bytes"
	"context"
	"errors"
	"log"
	"os"
	"sort"
	"time"

	"gopkg.in/yaml.v3"
)

const maxMessageBuffer = 100

var writeFrequency = time.Second * 5

func newFileBasedHistLogger(fname string) *fHistLogger {
	if fname == "" {
		return nil
	}
	blocked, accepted := parseHistogramFile(fname)
	ctx, cancel := context.WithCancel(context.Background())
	result := &fHistLogger{
		fname:                       fname,
		ch:                          make(chan message, maxMessageBuffer),
		blocked:                     blocked,
		accepted:                    accepted,
		stopGeneratingWriteWorkload: cancel,
	}
	go result.generateWriteWorkload(ctx)
	go result.run()
	return result
}

type fHistLogger struct {
	fname string
	// internal
	ch                          chan message
	closed                      bool
	modified                    int
	blocked                     map[string]int
	accepted                    map[string]int
	stopGeneratingWriteWorkload context.CancelFunc
}

type histContent struct {
	Blocked  []fqdnDetails
	Accepted []fqdnDetails
}

type fqdnDetails struct {
	FQDN  string
	Count int
}

func newHistContent(blocked map[string]int, accepted map[string]int) histContent {
	return histContent{
		Blocked:  newFQDNDetails(blocked),
		Accepted: newFQDNDetails(accepted),
	}
}

func newFQDNDetails(input map[string]int) []fqdnDetails {
	result := make([]fqdnDetails, 0, len(input))
	for k, v := range input {
		result = append(result, fqdnDetails{FQDN: k, Count: v})
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].Count > result[j].Count
	})
	return result
}

func parseHistogramFile(fname string) (map[string]int, map[string]int) {
	blocked := make(map[string]int)
	accepted := make(map[string]int)
	contents, err := os.ReadFile(fname)
	if err != nil {
		log.Printf("Error reading histogram file %s: %v", fname, err)
		return blocked, accepted
	}
	buffer := histContent{}
	if err := yaml.Unmarshal(contents, &buffer); err != nil {
		log.Printf("Error reading histogram file %s: %v", fname, err)
		return blocked, accepted
	}
	for _, v := range buffer.Blocked {
		blocked[v.FQDN] = v.Count
	}
	for _, v := range buffer.Accepted {
		accepted[v.FQDN] = v.Count
	}
	return blocked, accepted
}

func (fhl *fHistLogger) run() {
	for {
		msg := <-fhl.ch
		switch msg.messageType() {
		case logAcceptedAsynchMessageType:
			fhl.processLogAcceptedMessage(msg.request().(requestMessage[string, struct{}]))
		case logBlockedAsyncMessageType:
			fhl.processLogBlockedMessage(msg.request().(requestMessage[string, struct{}]))
		case writeMessageType:
			incoming := msg.request().(requestMessage[struct{}, struct{}])
			if !fhl.closed {
				fhl.processWriteMessage(incoming)
			} else {
				incoming.resp <- responsePayloadWithError[struct{}]{
					err: errors.New("logger already closed"),
				}
			}
		case closeAsynchMessageType:
			incoming := msg.request().(requestMessage[struct{}, struct{}])
			if !fhl.closed {
				fhl.stopGeneratingWriteWorkload()
				fhl.closed = true
				fhl.processWriteMessage(msg.request().(requestMessage[struct{}, struct{}]))
			} else {
				close(incoming.resp)
			}
		default:
		}
	}
}

func (fhl *fHistLogger) generateWriteWorkload(ctx context.Context) {
	defer func() {
		log.Println("Histogram Logger Write Workload Generator finished...")
	}()
	for {
		select {
		case <-time.After(writeFrequency):
			resp := make(chan responsePayloadWithError[struct{}])
			fhl.ch <- asynchMessage[struct{}, struct{}]{
				mType: writeMessageType,
				req: requestMessage[struct{}, struct{}]{
					resp: resp,
				},
			}
			select {
			case v := <-resp:
				if v.err != nil {
					log.Printf("Error writing to histogram logger: %v", v.err)
				}
			case <-ctx.Done():
				return
			}
		case <-ctx.Done():
			return
		}
	}
}

func (fhl *fHistLogger) Close() error {
	if fhl == nil {
		return nil
	}
	resp := make(chan responsePayloadWithError[struct{}])
	fhl.ch <- asynchMessage[struct{}, struct{}]{
		mType: closeAsynchMessageType,
		req: requestMessage[struct{}, struct{}]{
			resp: resp,
		},
	}
	v := <-resp
	if v.err != nil {
		log.Printf("Error writing to histogram logger while closing: %v", v.err)
		return v.err
	}
	return nil
}

func (fhl *fHistLogger) LogAccepted(fqdn string) {
	if fhl == nil {
		return
	}
	resp := make(chan responsePayloadWithError[struct{}])
	fhl.ch <- asynchMessage[string, struct{}]{
		mType: logAcceptedAsynchMessageType,
		req: requestMessage[string, struct{}]{
			req:  fqdn,
			resp: resp,
		},
	}
	<-resp
}

func (fhl *fHistLogger) LogBlocked(fqdn string) {
	if fhl == nil {
		return
	}
	resp := make(chan responsePayloadWithError[struct{}])
	fhl.ch <- asynchMessage[string, struct{}]{
		mType: logBlockedAsyncMessageType,
		req: requestMessage[string, struct{}]{
			req:  fqdn,
			resp: resp,
		},
	}
	<-resp
}

func (fhl *fHistLogger) processLogAcceptedMessage(msg requestMessage[string, struct{}]) {
	defer close(msg.resp)
	if fhl.closed {
		return
	}
	fhl.accepted[msg.req] = fhl.accepted[msg.req] + 1
	fhl.modified++
}

func (fhl *fHistLogger) processLogBlockedMessage(msg requestMessage[string, struct{}]) {
	defer close(msg.resp)
	if fhl.closed {
		return
	}
	fhl.blocked[msg.req] = fhl.blocked[msg.req] + 1
	fhl.modified++
}

func (fhl *fHistLogger) processWriteMessage(msg requestMessage[struct{}, struct{}]) {
	if fhl.modified == 0 {
		// log.Printf("Histogram logger save skipping... no changes...")
		close(msg.resp)
		return
	}
	content := newHistContent(fhl.blocked, fhl.accepted)
	toBeModified := fhl.modified
	// NOTE: WARNING: if write fails it will not be attempted again because modified flag is reset!
	// We're willing to lose data for efficiency
	fhl.modified = 0
	go fhl.write(content, msg.resp, toBeModified)
}

func (fhl *fHistLogger) write(content histContent, resp chan responsePayloadWithError[struct{}], numModified int) {
	contentBytes := bytes.Buffer{}
	enc := yaml.NewEncoder(&contentBytes)
	enc.SetIndent(2)
	if err := enc.Encode(content); err != nil {
		resp <- responsePayloadWithError[struct{}]{
			err: err,
		}
		return
	}
	if err := os.WriteFile(fhl.fname, contentBytes.Bytes(), 0644); err != nil {
		resp <- responsePayloadWithError[struct{}]{
			err: err,
		}
		return
	}
	close(resp)
	log.Printf("Histogram logger saved %d entries...", numModified)
}

type asynchMessageType uint8

const (
	undefinedAsyncMessageType asynchMessageType = iota
	logAcceptedAsynchMessageType
	logBlockedAsyncMessageType
	writeMessageType
	closeAsynchMessageType
)

type message interface {
	messageType() asynchMessageType
	request() interface{}
}

type requestPayload interface {
	struct{} | string
}

type responsePayload interface {
	struct{} | string
}

type responsePayloadWithError[T responsePayload] struct {
	payload T
	err     error
}

type requestMessage[T1 requestPayload, T2 responsePayload] struct {
	req  T1
	resp chan responsePayloadWithError[T2]
}

type asynchMessage[T1 requestPayload, T2 responsePayload] struct {
	mType asynchMessageType
	req   requestMessage[T1, T2]
}

func (msg asynchMessage[T1, T2]) messageType() asynchMessageType {
	return msg.mType
}

func (msg asynchMessage[T1, T2]) request() interface{} {
	return msg.req
}
