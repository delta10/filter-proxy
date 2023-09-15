package logs

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/delta10/filter-proxy/internal/config"
)

func NewLogBackend(backend config.LogBackend) *LogBackend {
	return &LogBackend{
		Config: backend,
	}
}

type LogBackend struct {
	Config config.LogBackend
}

type Stream struct {
	Stream map[string]string `json:"stream"`
	Values [][]any           `json:"values"`
}

type Body struct {
	Streams []Stream `json:"streams"`
}

func (l *LogBackend) WriteLog(labels map[string]string, line map[string]string) error {
	parsedUrl, err := url.Parse(l.Config.BaseURL)
	if err != nil {
		return err
	}

	parsedUrl = parsedUrl.JoinPath("/api/v1/push")

	marshalledLine, err := json.Marshal(line)
	if err != nil {
		return err
	}

	body := Body{
		Streams: []Stream{
			{
				Stream: labels,
				Values: [][]any{
					{
						fmt.Sprint(time.Now().UnixNano()),
						string(marshalledLine),
					},
				},
			},
		},
	}

	marshalled, err := json.Marshal(body)
	if err != nil {
		return err
	}

	logRequest, err := http.NewRequest("POST", parsedUrl.String(), bytes.NewReader(marshalled))
	if err != nil {
		return err
	}

	logRequest.Header.Add("Content-Type", "application/json")

	client := &http.Client{}
	logResponse, err := client.Do(logRequest)
	if err != nil {
		return err
	}

	defer logResponse.Body.Close()

	if logResponse.StatusCode != http.StatusNoContent {
		return errors.New("could not create log entry")
	}

	return nil
}
