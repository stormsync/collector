package main

//
// import (
// 	"context"
// 	"fmt"
// 	"log"
// 	"net/http"
// 	"net/http/httptest"
// 	"os"
// 	"testing"
// )
//
// func mustReadFile(fn string) string {
// 	var f []byte
// 	var err error
// 	if f, err = os.ReadFile(fn); err != nil {
// 		log.Fatal(err)
// 	}
// 	return string(f)
//
// }
// func TestCollector_Collect(t *testing.T) {
// 	type fields struct {
// 		kafkaWriter    *kafka.Writer
// 		collectionURLs []string
// 		topic          string
// 		balancer       kafka.Balancer
// 	}
//
// 	type args struct {
// 		ctx context.Context
// 	}
//
// 	tests := []struct {
// 		name     string
// 		testFile string
// 		fields   fields
// 		args     args
// 		wantErr  bool
// 	}{
// 		{
// 			name:     "should process response correctly",
// 			testFile: mustReadFile("./test_data/filtered_hail.csv"),
// 			fields: fields{
// 				kafkaWriter:    nil,
// 				collectionURLs: nil,
// 				topic:          "",
// 			},
// 			args: args{ctx: context.Background()},
// 		},
// 	}
//
// 	for _, tt := range tests {
// 		t.Run(tt.name, func(t *testing.T) {
// 			svr := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
// 				fmt.Fprintf(w, tt.testFile)
// 			}))
// 			defer svr.Close()
//
// 			addr := kafka.TCP("localhost:9092")
//
// 			tt.fields.collectionURLs = append(tt.fields.collectionURLs, svr.URL)
// 			c := NewCollector(addr, tt.fields.topic, tt.fields.balancer, tt.fields.collectionURLs)
// 			if err := c.CollectAndPublish(tt.args.ctx); (err != nil) != tt.wantErr {
// 				t.Errorf("Collect() error = %v, wantErr %v", err, tt.wantErr)
// 			}
//
// 		})
// 	}
// }
