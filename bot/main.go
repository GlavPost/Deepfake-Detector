package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

type AnalysisResult struct {
	IsDeepfake bool    `json:"is_deepfake"`
	Confidence float64 `json:"confidence"`
}

func isImageDocument(doc *tgbotapi.Document) bool {
	if doc == nil {
		return false
	}

	ext := strings.ToLower(filepath.Ext(doc.FileName))
	imageExts := map[string]bool{
		".jpg": true, ".jpeg": true, ".png": true,
		".gif": true, ".webp": true, ".bmp": true,
	}
	return imageExts[ext]
}

// getFileFromMessage извлекает файл (фото или документ-изображение) из сообщения
// Возвращает: фото (если есть), документ (если есть), является ли это изображением
func getFileFromMessage(msg *tgbotapi.Message) (*tgbotapi.PhotoSize, *tgbotapi.Document, bool) {
	// Случай 1: отправлено как фото
	if len(msg.Photo) > 0 {
		// Берём фото с максимальным разрешением
		photo := msg.Photo[len(msg.Photo)-1]
		return &photo, nil, true
	}

	// Случай 2: отправлено как документ
	if msg.Document != nil && isImageDocument(msg.Document) {
		return nil, msg.Document, true
	}

	// Не изображение
	return nil, nil, false
}

func main() {
	botToken := os.Getenv("BOT_TOKEN")
	if botToken == "" {
		log.Fatal("BOT_TOKEN environment variable is required")
	}

	gatewayURL := os.Getenv("GATEWAY_URL")
	if gatewayURL == "" {
		gatewayURL = "http://gateway:8080"
	}

	bot, err := tgbotapi.NewBotAPI(botToken)
	if err != nil {
		log.Panicf("Failed to init bot: %v", err)
	}
	bot.Debug = false

	log.Printf("Authorized on account %s", bot.Self.UserName)

	commands := []tgbotapi.BotCommand{
		{Command: "start", Description: "Запустить бота"},
		{Command: "help", Description: "Как пользоваться"},
	}
	_, _ = bot.Request(tgbotapi.NewSetMyCommands(commands...))

	u := tgbotapi.NewUpdate(0)
	u.Timeout = 30
	updates := bot.GetUpdatesChan(u)

	httpClient := &http.Client{Timeout: 45 * time.Second}

	for update := range updates {
		if update.Message == nil {
			continue
		}

		chatID := update.Message.Chat.ID

		if update.Message.IsCommand() {
			switch update.Message.Command() {
			case "start":
				sendStartMessage(bot, chatID)
			case "help":
				_, _ = bot.Send(tgbotapi.NewMessage(chatID, "📤 Отправьте фото или файл с изображением.\n📱 Или нажмите кнопку для Mini App."))
			default:
				_, _ = bot.Send(tgbotapi.NewMessage(chatID, "❓ Неизвестная команда. Используйте /start или /help"))
			}
			continue
		}

		photo, doc, isImage := getFileFromMessage(update.Message)

		if isImage {
			go analyzeAndReply(bot, chatID, photo, doc, gatewayURL, httpClient)
			continue
		}

		// Реакция на неподдерживаемый контент
		// Игнорируем системные сообщения и ответы бота, чтобы не спамить
		if !update.Message.IsCommand() {
			continue
		}

		_, _ = bot.Send(tgbotapi.NewMessage(chatID, "🤔 Я понимаю только изображения (JPG, PNG, WebP, GIF).\n\n📤 Отправьте фото или файл с картинкой для анализа."))
	}
}

func sendStartMessage(bot *tgbotapi.BotAPI, chatID int64) {
	msg := tgbotapi.NewMessage(chatID, "🕵️‍♂️ *DeepDetect Bot*\n\nОтправьте изображение для проверки.\nИли откройте Mini App для расширенного интерфейса.")
	msg.ParseMode = "Markdown"

	miniAppURL := os.Getenv("MINI_APP_URL")
	if miniAppURL == "" {
		miniAppURL = "https://fallback-url.com"
	}

	msg.ReplyMarkup = map[string]interface{}{
		"inline_keyboard": [][]map[string]interface{}{
			{
				{
					"text":    "🌐 Открыть Mini App",
					"web_app": map[string]string{"url": miniAppURL},
				},
			},
		},
	}

	_, _ = bot.Send(msg)
}

func analyzeAndReply(bot *tgbotapi.BotAPI, chatID int64, photo *tgbotapi.PhotoSize, doc *tgbotapi.Document, gatewayURL string, client *http.Client) {
	_, _ = bot.Send(tgbotapi.NewChatAction(chatID, tgbotapi.ChatTyping))

	var fileID string
	var fileName string

	if photo != nil {
		fileID = photo.FileID
		fileName = "photo.jpg"
	} else if doc != nil {
		fileID = doc.FileID
		fileName = doc.FileName
		if fileName == "" {
			fileName = "image_file"
		}
	} else {
		replyWithError(bot, chatID, "❌ Не удалось определить файл")
		return
	}

	tgFile, err := bot.GetFile(tgbotapi.FileConfig{FileID: fileID})
	if err != nil {
		log.Printf("Failed to get file info: %v", err)
		replyWithError(bot, chatID, "❌ Не удалось скачать изображение")
		return
	}

	downloadURL := fmt.Sprintf("https://api.telegram.org/file/bot%s/%s", bot.Token, tgFile.FilePath)

	resp, err := http.Get(downloadURL)
	if err != nil {
		log.Printf("Failed to download file: %v", err)
		replyWithError(bot, chatID, "❌ Ошибка загрузки файла")
		return
	}
	defer resp.Body.Close()

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	part, err := writer.CreateFormFile("image", fileName)
	if err != nil {
		log.Printf("Failed to create form file: %v", err)
		replyWithError(bot, chatID, "⚠️ Ошибка подготовки запроса")
		return
	}

	_, err = io.Copy(part, resp.Body)
	if err != nil {
		log.Printf("Failed to copy file content: %v", err)
		replyWithError(bot, chatID, "⚠️ Ошибка обработки файла")
		return
	}

	err = writer.Close()
	if err != nil {
		log.Printf("Failed to close writer: %v", err)
		replyWithError(bot, chatID, "⚠️ Ошибка формирования запроса")
		return
	}

	req, err := http.NewRequest("POST", gatewayURL+"/api/analyze", body)
	if err != nil {
		log.Printf("Failed to create request: %v", err)
		replyWithError(bot, chatID, "⚠️ Внутренняя ошибка")
		return
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())

	gwResp, err := client.Do(req)
	if err != nil {
		log.Printf("Gateway request failed: %v", err)
		replyWithError(bot, chatID, "⚠️ Сервер анализа перегружен. Попробуйте позже.")
		return
	}
	defer gwResp.Body.Close()

	if gwResp.StatusCode != http.StatusOK {
		log.Printf("Gateway returned status: %d", gwResp.StatusCode)
		replyWithError(bot, chatID, "⚠️ Ошибка при анализе. Попробуйте другое изображение.")
		return
	}

	var result AnalysisResult
	if err := json.NewDecoder(gwResp.Body).Decode(&result); err != nil {
		log.Printf("Failed to decode response: %v", err)
		replyWithError(bot, chatID, "⚠️ Ошибка обработки результата")
		return
	}

	emoji, status := "✅", "ИЗОБРАЖЕНИЕ ПОДЛИННОЕ"
	if result.IsDeepfake {
		emoji, status = "⚠️", "ОБНАРУЖЕН DEEPFAKE"
	}

	msg := tgbotapi.NewMessage(chatID,
		fmt.Sprintf("%s *%s*\n📊 Уверенность: **%.1f%%**",
			emoji, status, result.Confidence*100))
	msg.ParseMode = "Markdown"
	_, _ = bot.Send(msg)
}

func replyWithError(bot *tgbotapi.BotAPI, chatID int64, text string) {
	_, _ = bot.Send(tgbotapi.NewMessage(chatID, text))
}
