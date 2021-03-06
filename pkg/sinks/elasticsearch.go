package sinks

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"github.com/elastic/go-elasticsearch/v7"
	"github.com/elastic/go-elasticsearch/v7/esapi"
	"github.com/opsgenie/kubernetes-event-exporter/pkg/kube"
	"io/ioutil"
	"net/http"
	"regexp"
	"strings"
	"time"
)

type ElasticsearchConfig struct {
	// Connection specific
	Hosts    []string `yaml:"hosts"`
	Username string   `yaml:"username"`
	Password string   `yaml:"password"`
	CloudID  string   `yaml:"cloudID"`
	APIKey   string   `yaml:"apiKey"`
	// Indexing preferences
	UseEventID  bool   `yaml:"useEventID"`
	Index       string `yaml:"index"`
	IndexFormat string `yaml:"indexFormat"`
	TLS         struct {
		InsecureSkipVerify bool   `yaml:"insecureSkipVerify"`
		ServerName         string `yaml:"serverName"`
		CaFile             string `yaml:"caFile"`
	} `yaml:"tls"`
}

func NewElasticsearch(cfg *ElasticsearchConfig) (*Elasticsearch, error) {
	var caCert []byte

	if len(cfg.TLS.CaFile) > 0 {
		readFile, err := ioutil.ReadFile(cfg.TLS.CaFile)
		if err != nil {
			return nil, err
		}
		caCert = readFile
	}

	tlsClientConfig := &tls.Config{
		InsecureSkipVerify: cfg.TLS.InsecureSkipVerify,
		ServerName:         cfg.TLS.ServerName,
	}
	tlsClientConfig.RootCAs.AppendCertsFromPEM(caCert)

	client, err := elasticsearch.NewClient(elasticsearch.Config{
		Addresses: cfg.Hosts,
		Username:  cfg.Username,
		Password:  cfg.Password,
		CloudID:   cfg.CloudID,
		APIKey:    cfg.APIKey,
		Transport: &http.Transport{
			TLSClientConfig: tlsClientConfig,
		},
	})
	if err != nil {
		return nil, err
	}

	return &Elasticsearch{
		client: client,
		cfg:    cfg,
	}, nil
}

type Elasticsearch struct {
	client *elasticsearch.Client
	cfg    *ElasticsearchConfig
}

var regex = regexp.MustCompile(`(?s){(.*)}`)

func formatIndexName(pattern string, when time.Time) string {
	m := regex.FindAllStringSubmatchIndex(pattern, -1)
	current := 0
	var builder strings.Builder

	for i := 0; i < len(m); i++ {
		pair := m[i]

		builder.WriteString(pattern[current:pair[0]])
		builder.WriteString(when.Format(pattern[pair[0]+1 : pair[1]-1]))
		current = pair[1]
	}

	builder.WriteString(pattern[current:])

	return builder.String()
}

func (e *Elasticsearch) Send(ctx context.Context, ev *kube.EnhancedEvent) error {
	b, err := json.Marshal(ev)
	if err != nil {
		return err
	}

	var index string
	if len(e.cfg.IndexFormat) > 0 {
		now := time.Now()
		index = formatIndexName(e.cfg.IndexFormat, now)
	} else {
		index = e.cfg.Index
	}

	req := esapi.IndexRequest{
		Body:  bytes.NewBuffer(b),
		Index: index,
	}

	if e.cfg.UseEventID {
		req.DocumentID = string(ev.UID)
	}

	resp, err := req.Do(ctx, e.client)
	if err != nil {
		return err
	}

	defer resp.Body.Close()
	_ = resp.Body
	return nil
}

func (e *Elasticsearch) Close() {
	// No-op
}
