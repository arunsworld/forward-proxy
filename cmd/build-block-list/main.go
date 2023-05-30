package main

import (
	"bytes"
	"encoding/csv"
	"io"
	"log"
	"net/http"
	"os"

	"github.com/urfave/cli/v2"
	"gopkg.in/yaml.v3"
)

type blockConfig struct {
	BlockList   map[string][]string
	BlockSuffix []string
}

func main() {
	var staticFileName string
	var outFileName string
	app := &cli.App{
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:        "static",
				Value:       "static.yml",
				Destination: &staticFileName,
			},
			&cli.StringFlag{
				Name:        "output",
				Value:       "fqdn-block.yml",
				Destination: &outFileName,
			},
		},
		Action: func(cCtx *cli.Context) error {
			return prepareFQDNBlockOutput(staticFileName, outFileName)
		},
	}
	if err := app.Run(os.Args); err != nil {
		log.Fatal(err)
	}
}

func prepareFQDNBlockOutput(staticFileName, outFileName string) error {
	staticContents, err := os.ReadFile(staticFileName)
	if err != nil {
		return err
	}
	data := blockConfig{}
	if err := yaml.Unmarshal(staticContents, &data); err != nil {
		return err
	}

	url := "https://www.github.developerdan.com/hosts/lists/ads-and-tracking-extended.txt"
	v, err := readRemoteIPHostFile(url, ' ', '#')
	if err != nil {
		return err
	}
	data.BlockList[url] = v

	output, err := yaml.Marshal(data)
	if err != nil {
		return err
	}
	return os.WriteFile(outFileName, output, 0644)
}

func readRemoteIPHostFile(url string, comma, comment rune) ([]string, error) {
	log.Printf("Reading from URL: %s", url)
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	contents, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	log.Printf("Read %d bytes", len(contents))
	r := csv.NewReader(bytes.NewReader(contents))
	r.Comma = comma
	r.TrimLeadingSpace = true
	r.LazyQuotes = true
	r.FieldsPerRecord = 2
	r.Comment = comment

	result := []string{}
	for {
		record, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			log.Printf("\terror while reading: %v", err)
			continue
		}
		result = append(result, record[1])
	}
	return result, nil
}
