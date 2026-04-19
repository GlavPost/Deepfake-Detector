package main

import (
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/segmentio/kafka-go"
)

// Структуры для сообщений
type AnalysisRequest struct {
	RequestID string `json:"request_id"`
	FileName  string `json:"file_name"`
}

type AnalysisResult struct {
	RequestID  string  `json:"request_id"`
	IsDeepfake bool    `json:"is_deepfake"`
	Confidence float64 `json:"confidence"`
}

var (
	// Карта для хранения каналов ответов: RequestID -> chan AnalysisResult
	pendingRequests sync.Map
	kafkaWriter     *kafka.Writer
)

func main() {
	log.SetFlags(log.Ldate | log.Ltime | log.Lshortfile)

	// 1. Инициализация Kafka Writer
	kafkaWriter = &kafka.Writer{
		Addr:     kafka.TCP("kafka:9092"),
		Balancer: &kafka.LeastBytes{},
	}

	// 2. Запуск фонового слушателя ответов
	go responseListener()

	// 3. Маршруты
	// Раздача фронтенда
	fs := http.FileServer(http.Dir("./static"))
	http.Handle("/", fs)

	// API для анализа
	http.HandleFunc("/api/analyze", handleAnalyze)

	log.Println("Gateway запущен на :8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}

func handleAnalyze(w http.ResponseWriter, r *http.Request) {

	if r.Method != http.MethodPost {
		log.Println("WARNING: Invalid method attempt:", r.Method)
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Читаем файл из формы
	file, header, err := r.FormFile("image")
	if err != nil {
		log.Printf("ERROR: Failed to parse form file: %v", err)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	defer file.Close()

	// Создаем уникальный ID и сохраняем файл
	requestID := uuid.New().String()
	log.Printf("[%s] NEW REQUEST: Filename=%s", requestID, header.Filename)

	ext := filepath.Ext(header.Filename)
	fileName := requestID + ext
	filePath := filepath.Join("./shared_data", fileName)

	out, err := os.Create(filePath)
	if err != nil {
		log.Printf("[%s] ERROR: Could not create file on disk: %v", requestID, err)
		http.Error(w, "Failed to save image", http.StatusInternalServerError)
		return
	}
	defer out.Close()
	io.Copy(out, file)
	log.Printf("[%s] DISK IO: File successfully written to %s", requestID, filePath)

	// Создаем канал для ожидания ответа
	resChan := make(chan AnalysisResult)
	pendingRequests.Store(requestID, resChan)
	defer pendingRequests.Delete(requestID)

	// Определяем топик: если PNG, сразу в images_ready
	topic := "images_raw"
	if ext == ".png" {
		topic = "images_ready"
		log.Printf("[%s] ROUTING: PNG detected, skipping converter, routing to %s", requestID, topic)
	}

	// Отправляем в Kafka
	msg := AnalysisRequest{RequestID: requestID, FileName: fileName}
	msgBytes, _ := json.Marshal(msg)

	log.Printf("[%s] KAFKA PRODUCER: Sending to topic '%s'", requestID, topic)
	err = kafkaWriter.WriteMessages(context.Background(), kafka.Message{
		Topic: topic,
		Value: msgBytes,
	})

	if err != nil {
		log.Printf("[%s] KAFKA ERROR: Failed to publish message: %v", requestID, err)
		http.Error(w, "Internal Queue Error", http.StatusInternalServerError)
		return
	}
	log.Printf("[%s] KAFKA: Message published to %s. Waiting for response...", requestID, topic)

	// Ждем ответ из канала
	select {
	case result := <-resChan:
		log.Printf("[%s] SUCCESS: Result received from Kafka. IsDeepfake: %v", requestID, result.IsDeepfake)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(result)
	case <-time.After(30 * time.Second):
		log.Printf("[%s] TIMEOUT: No response from detectors after 30s", requestID)
		http.Error(w, "Processing timeout", http.StatusGatewayTimeout)
	case <-r.Context().Done(): // Если пользователь закрыл вкладку
		return
	}
}

func responseListener() {
	log.Println("SYSTEM: Kafka Response Listener started")

	reader := kafka.NewReader(kafka.ReaderConfig{
		Brokers: []string{"kafka:9092"},
		Topic:   "results",
		GroupID: "gateway-group",
	})

	for {
		m, err := reader.ReadMessage(context.Background())
		if err != nil {
			log.Printf("Reader error: %v", err)
			continue
		}

		var res AnalysisResult
		json.Unmarshal(m.Value, &res)
		log.Printf("[%s] KAFKA CONSUMER: Captured result from 'results' topic", res.RequestID)

		// Ищем канал ожидания и отправляем туда результат
		if val, ok := pendingRequests.Load(res.RequestID); ok {
			resChan := val.(chan AnalysisResult)
			resChan <- res
		}
	}
}
