package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"sync"
	"time"
)

var (
	lastSMSTime time.Time
	mutex       sync.Mutex
)

type SMSRequest struct {
	OtpID        string   `json:"OtpId"`
	ReplaceToken []string `json:"ReplaceToken"`
	SenderNumber string   `json:"SenderNumber"`
	MobileNumber string   `json:"MobileNumber"`
}

var (
	sender   = "10000000002027"
	receiver = "09024809750"
	smsKey   = "your_sms_api_key"
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

	sendSmsNow := false
	for name, durations := range transactionDurations {
		var totalDuration float64
		for _, duration := range durations {
			totalDuration += duration
		}
		averageDuration := totalDuration / float64(len(durations))
		if averageDuration > 500 {
			sendSmsNow = true
			log.Printf("Transaction Name: %s, Average Latency: %.2f ms\n", name, averageDuration)
		}
	}

	if sendSmsNow {
		sendSMS("673")
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
func sendSMS(code string) {
	mutex.Lock()
	defer mutex.Unlock()

	currentTime := time.Now()

	// Check if the last SMS was sent within the last 30 minutes
	if currentTime.Sub(lastSMSTime) < 30*time.Minute {
		fmt.Println("SMS not sent. Last SMS was sent less than 30 minutes ago.")
		return
	}

	url := "https://api.limosms.com/api/sendpatternmessage"
	smsRequest := SMSRequest{
		OtpID:        code,
		ReplaceToken: []string{},
		SenderNumber: sender,
		MobileNumber: receiver,
	}

	jsonData, err := json.Marshal(smsRequest)
	if err != nil {
		fmt.Printf("Failed to marshal JSON: %v\n", err)
		return
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		fmt.Printf("Failed to create request: %v\n", err)
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("ApiKey", smsKey)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		fmt.Printf("Failed to send SMS: %v\n", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		fmt.Println("SMS sent successfully")
		lastSMSTime = currentTime // Update the last SMS send time
	} else {
		fmt.Printf("Failed to send SMS. Status code: %d\n", resp.StatusCode)
	}
}
