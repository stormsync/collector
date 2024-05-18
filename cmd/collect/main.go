package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"strconv"
	"time"
)

func main() {
	var (
		WindURL    = "https://www.spc.noaa.gov/climo/reports/today_wind.csv"
		HailURL    = "https://www.spc.noaa.gov/climo/reports/today_hail.csv"
		TornadoURL = "https://www.spc.noaa.gov/climo/reports/today_torn.csv"
	)

	address := os.Getenv("KAFKA_ADDRESS")
	if address == "" {
		log.Fatal("address is required.  Use env var ADDRESS")
	}

	user := os.Getenv("KAFKA_USER")
	if user == "" {
		log.Fatal("kafka user is required.  Use env var KAFKA_USER")
	}
	fmt.Println(user)
	pw := os.Getenv("KAFKA_PASSWORD")
	if pw == "" {
		log.Fatal("kafka password is required.  Use env var KAFKA_PASSWORD")
	}
	fmt.Println(pw)
	topic := os.Getenv("TOPIC")
	if topic == "" {
		log.Fatal("topic is required.  Use env var TOPIC")
	}
	intervalStr := os.Getenv("POLLING_INTERVAL_IN_SECONDS")
	if intervalStr == "" {
		log.Fatal("polling interval is required.  Use env var POLLING_INTERVAL_IN_SECONDS")
	}
	_, err := strconv.Atoi(intervalStr)
	if err != nil {
		log.Fatal("interval needs to be an integer in quotes")
	}
	fmt.Println(intervalStr)
	intervalDuration, err := time.ParseDuration(intervalStr + "s")
	if err != nil {
		log.Fatal("only integers, representing seconds, are allowed for interval")
	}

	urlMap := map[ReportType]string{
		Wind:    WindURL,
		Hail:    HailURL,
		Tornado: TornadoURL,
	}

	collector, err := NewCollector(address, topic, user, pw, urlMap)
	if err != nil {
		log.Fatal("failed to create the collect: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	log.Printf("Starting collect service with %s interval", intervalDuration.String())
	collector.Poll(ctx, intervalDuration)
}
