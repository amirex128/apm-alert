package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"time"
)

type APMResponse struct {
	Hits struct {
		Hits []struct {
			Source struct {
				Transaction struct {
					Name              string    `json:"name"`
					DurationHistogram Histogram `json:"duration.histogram"`
				} `json:"transaction"`
			} `json:"_source"`
		} `json:"hits"`
	} `json:"hits"`
}

type Histogram struct {
	Values []float64 `json:"values"`
}

// 192.168.200.184:9200
const queryURL = "http://192.168.1.100:9299/apm-7.17.10-metric-2024.07.12/_search"

func main() {
	for {
		now := time.Now()
		log.Println("Monitoring APM Started at:", now)
		err := monitorAPM()
		log.Println("Monitoring APM Finished...")
		if err != nil {
			log.Println("Error monitoring APM:", err)
		}
		time.Sleep(5 * time.Minute)
	}
}

func monitorAPM() error {
	query := createElasticQuery()
	req, err := http.NewRequest("POST", queryURL, bytes.NewBuffer(query))
	if err != nil {
		return fmt.Errorf("error creating HTTP request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("error making HTTP request: %v", err)
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("error reading HTTP response: %v", err)
	}

	var apmResponse APMResponse
	err = json.Unmarshal(body, &apmResponse)
	if err != nil {
		return fmt.Errorf("error unmarshalling JSON: %v", err)
	}

	transactionDurations := make(map[string][]float64)

	for _, hit := range apmResponse.Hits.Hits {
		name := hit.Source.Transaction.Name
		for _, duration := range hit.Source.Transaction.DurationHistogram.Values {
			durationMs := duration / 1000 // Convert microseconds to milliseconds
			transactionDurations[name] = append(transactionDurations[name], durationMs)
		}
	}

	for name, durations := range transactionDurations {
		var totalDuration float64
		for _, duration := range durations {
			totalDuration += duration
		}
		averageDuration := totalDuration / float64(len(durations))
		if averageDuration > 500 {
			log.Printf("Transaction Name: %s, Average Latency: %.2f ms\n", name, averageDuration)
		}
	}

	return nil
}

func createElasticQuery() []byte {
	location, err := time.LoadLocation("Asia/Tehran")
	if err != nil {
		log.Fatalf("Error loading time zone: %v", err)
	}
	now := time.Now().In(location)
	fiveMinutesAgo := now.Add(-5 * time.Minute)

	query := map[string]interface{}{
		"query": map[string]interface{}{
			"bool": map[string]interface{}{
				"filter": []map[string]interface{}{
					{
						"range": map[string]interface{}{
							"@timestamp": map[string]interface{}{
								"gte": fiveMinutesAgo.Format(time.RFC3339),
								"lte": now.Format(time.RFC3339),
							},
						},
					},
					{
						"term": map[string]interface{}{
							"service.name": "production-search-afra",
						},
					},
				},
			},
		},
		"_source": []string{"transaction.name", "transaction.duration.histogram.values"},
		"size":    20000,
	}

	queryJSON, _ := json.Marshal(query)
	return queryJSON
}
