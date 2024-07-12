package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/joho/godotenv"
	_ "time/tzdata" // Import tzdata to embed timezone data
)

const alertDuration = 2000

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
	sender       string
	receiver     string
	smsKey       string
	queryURL     string
	indexPattern string
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

func init() {
	err := godotenv.Load()
	if err != nil {
		log.Fatalf("Error loading .env file")
	}

	smsKey = os.Getenv("SMS_KEY")
	if smsKey == "" {
		log.Fatalf("SMS_KEY not set in .env file")
	}

	sender = os.Getenv("SENDER_NUMBER")
	if sender == "" {
		log.Fatalf("SENDER_NUMBER not set in .env file")
	}

	receiver = os.Getenv("RECEIVER_NUMBER")
	if receiver == "" {
		log.Fatalf("RECEIVER_NUMBER not set in .env file")
	}

	queryURL = os.Getenv("QUERY_URL")
	if queryURL == "" {
		log.Fatalf("QUERY_URL not set in .env file")
	}

	indexPattern = os.Getenv("INDEX_PATTERN")
	if indexPattern == "" {
		log.Fatalf("INDEX_PATTERN not set in .env file")
	}
}

func main() {
	for {
		location, err := time.LoadLocation("Asia/Tehran")
		if err != nil {
			log.Fatalf("Error loading time zone: %v", err)
		}
		now := time.Now().In(location)

		if now.Hour() >= 7 && now.Hour() < 24 {
			log.Printf("Monitoring APM Started at: %s", now)
			err := monitorAPM()
			log.Println("Monitoring APM Finished...")
			if err != nil {
				log.Println("Error monitoring APM:", err)
			}
		} else {
			log.Printf("Current time %s is outside of monitoring hours. Skipping monitoring.", now)
			time.Sleep(25 * time.Minute)
		}

		time.Sleep(5 * time.Minute)
	}
}

func monitorAPM() error {
	query := createElasticQuery()
	log.Printf("Start With : %s/%s/_search\n", queryURL, indexPattern)
	fullQueryURL := fmt.Sprintf("%s/%s/_search", queryURL, indexPattern)
	req, err := http.NewRequest("POST", fullQueryURL, bytes.NewBuffer(query))
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
		if averageDuration > alertDuration {
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
		"size":    10000,
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
		log.Println("SMS not sent. Last SMS was sent less than 30 minutes ago.")
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
