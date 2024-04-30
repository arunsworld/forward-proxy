package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
)

type dnsAPIRequest struct {
	Addr string `json:"addr"`
	IP   string `json:"ip"`
}

func dnsHandler(dr dnsResolver) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprintf(w, "%s method not supported", r.Method)
			return
		}
		dec := json.NewDecoder(r.Body)
		var input []dnsAPIRequest
		if err := dec.Decode(&input); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprintf(w, "unable to parse request: %v", err)
			return
		}
		if len(input) == 0 {
			w.WriteHeader(http.StatusOK)
			fmt.Fprintf(w, `{"processed":0}`)
			return
		}
		for c, v := range input {
			if v.Addr == "" {
				w.WriteHeader(http.StatusBadRequest)
				fmt.Fprintf(w, "missing host address on index %d of input", c+1)
				return
			}
			if v.IP == "" {
				w.WriteHeader(http.StatusBadRequest)
				fmt.Fprintf(w, "missing IP address on index %d of input", c+1)
				return
			}
		}
		for _, v := range input {
			dr.Register(v.Addr, v.IP)
			log.Printf("registered: %s - %s", v.Addr, v.IP)
		}
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, `{"processed":%d}`, len(input))
	}
}

func dnsListHandler(dr dnsResolver) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, _ *http.Request) {
		reg := dr.Registrations()
		result := make([]dnsAPIRequest, 0, len(reg))
		for k, v := range reg {
			result = append(result, dnsAPIRequest{
				Addr: k,
				IP:   v.String(),
			})
		}
		v, err := json.Marshal(result)
		if err != nil {
			fmt.Fprintf(w, "error processing JSON: %v", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
		w.Write(v)
	}
}

type apiServer struct {
	hostname string
	port     int
	dr       dnsResolver
	srv      *http.Server
}

func (s *apiServer) serve() error {
	if s.port == 0 {
		return nil
	}
	s.dr.Register("api", "127.0.0.1")
	http.HandleFunc("/", dnsHandler(s.dr))
	http.HandleFunc("/list", dnsListHandler(s.dr))
	addr := fmt.Sprintf("%s:%d", s.hostname, s.port)
	srv := &http.Server{Addr: addr}
	s.srv = srv
	log.Printf("serving api server on address: %s", addr)
	defer func() {
		log.Println("api server terminated")
	}()
	return srv.ListenAndServe()
}

func (s *apiServer) close() {
	if s.port == 0 {
		return
	}
	s.srv.Close()
}
