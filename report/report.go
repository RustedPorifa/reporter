package report

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math/rand/v2"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"reporter/godb"
	"reporter/reader"
	"strconv"
	"strings"
	"time"

	"github.com/gotd/td/session"
	"github.com/gotd/td/telegram"
	"github.com/gotd/td/telegram/dcs"
	"github.com/gotd/td/tg"
	"golang.org/x/net/proxy"
)

type MessageData struct {
	Messages []string `json:"messages"`
}

var trashPath = filepath.Join("trash")

func IsValid(identifier string) bool {
	// Проверка на числовой ID
	if _, err := strconv.ParseInt(identifier, 10, 64); err == nil {
		return true
	}

	// Проверка username (без @ в начале)
	re := regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9_]{4,31}$`)
	return re.MatchString(identifier)
}

func StartReport(pathToFile string, identifier string, reportType string, fileName string) error {
	// Проверка идентификатора перед началом операции
	if !IsValid(identifier) {
		return fmt.Errorf("invalid identifier format: %s", identifier)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(rand.IntN(10)+5)*time.Minute)
	defer cancel()

	// Чтение сессионного файла
	dataBytes, err := os.ReadFile(pathToFile)
	if err != nil {
		return fmt.Errorf("error reading session file: %w", err)
	}

	// Инициализация dialer
	dialer, err := initProxyDialer(fileName)
	if err != nil {
		dialer = proxy.Direct
	}

	// Загрузка сессии
	storage, err := loadSession(dataBytes)
	if err != nil {
		log.Println(err)
	}

	// Создание клиента Telegram
	client := createTelegramClient(storage, dialer)

	// Выполнение операции репорта
	if err := client.Run(ctx, func(ctx context.Context) error {
		return performReport(ctx, client.API(), identifier, reportType)
	}); err != nil {
		log.Println(err)
		moveToTrash(pathToFile)
		return err
	}

	return nil
}

// Вспомогательные функции
func initProxyDialer(fileName string) (proxy.Dialer, error) {
	// Пытаемся получить прокси по имени
	proxyURL, err := godb.GetProxyByName(fileName)
	if err == nil && proxyURL != "" {
		return createProxyDialer(proxyURL)
	}

	// Получаем случайный прокси
	accValue, err1 := reader.GetReports()
	proxyCount, err2 := godb.GetProxyCount()

	value := 0
	if err1 == nil && err2 == nil && proxyCount > 0 {
		value = accValue / proxyCount
	}

	newProxy, err := godb.GetRandomProxyBelow(value)
	if err != nil {
		return proxy.Direct, nil // Используем прямое соединение как fallback
	}

	return createProxyDialer(newProxy)
}

func createProxyDialer(proxyURL string) (proxy.Dialer, error) {
	parts := strings.Split(proxyURL, ":")
	if len(parts) < 4 {
		return nil, errors.New("invalid proxy format")
	}

	addr := net.JoinHostPort(parts[0], parts[1])
	auth := proxy.Auth{User: parts[2], Password: parts[3]}

	dialer, err := proxy.SOCKS5("tcp", addr, &auth, proxy.Direct)
	if err != nil {
		return nil, fmt.Errorf("proxy creation error: %w", err)
	}
	return dialer, nil
}

func loadSession(dataBytes []byte) (*session.StorageMemory, error) {
	var sessionInfo session.Data
	if err := json.Unmarshal(dataBytes, &sessionInfo); err != nil {
		return nil, fmt.Errorf("session unmarshal error: %w", err)
	}

	storage := &session.StorageMemory{}
	loader := session.Loader{Storage: storage}
	if err := loader.Save(context.Background(), &sessionInfo); err != nil {
		return nil, fmt.Errorf("session load error: %w", err)
	}
	return storage, nil
}

func createTelegramClient(storage session.Storage, dialer proxy.Dialer) *telegram.Client {
	dialContext := func(ctx context.Context, network, addr string) (net.Conn, error) {
		return dialer.Dial(network, addr)
	}

	return telegram.NewClient(telegram.TestAppID, telegram.TestAppHash, telegram.Options{
		SessionStorage: storage,
		NoUpdates:      true,
		MaxRetries:     2,
		Resolver: dcs.Plain(dcs.PlainOptions{
			Dial: dialContext,
		}),
	})
}

func performReport(ctx context.Context, api *tg.Client, identifier, reportType string) error {
	// Попробуем обработать как числовой ID
	if id, err := strconv.ParseInt(identifier, 10, 64); err == nil {
		return reportByID(ctx, api, id, reportType)
	}

	// Обрабатываем как username (удаляем @ если есть)
	username := strings.TrimPrefix(identifier, "@")
	return reportByUsername(ctx, api, username, reportType)
}

func reportByUsername(ctx context.Context, api *tg.Client, username, reportType string) error {
	// Разрешение username
	resolved, err := api.ContactsResolveUsername(ctx, &tg.ContactsResolveUsernameRequest{
		Username: username,
	})
	if err != nil {
		return err
	}

	// Поиск целевого пользователя
	var targetPeer tg.InputPeerClass
	for _, user := range resolved.Users {
		if u, ok := user.(*tg.User); ok && strings.EqualFold(u.Username, username) {
			targetPeer = &tg.InputPeerUser{
				UserID:     u.ID,
				AccessHash: u.AccessHash,
			}
			break
		}
	}
	if targetPeer == nil {
		return errors.New("target user not found")
	}

	return sendReport(ctx, api, targetPeer, reportType)
}

func reportByID(ctx context.Context, api *tg.Client, userID int64, reportType string) error {
	// Получаем информацию о пользователе по ID
	users, err := api.UsersGetUsers(ctx, []tg.InputUserClass{
		&tg.InputUser{
			UserID: userID,
		},
	})
	if err != nil {
		return fmt.Errorf("failed to get user by ID: %w", err)
	}

	if len(users) == 0 {
		return errors.New("user not found by ID")
	}

	user, ok := users[0].(*tg.User)
	if !ok {
		return errors.New("invalid user object received")
	}

	targetPeer := &tg.InputPeerUser{
		UserID:     user.ID,
		AccessHash: user.AccessHash,
	}

	return sendReport(ctx, api, targetPeer, reportType)
}

func sendReport(ctx context.Context, api *tg.Client, peer tg.InputPeerClass, reportType string) error {
	// Загрузка сообщений для репорта
	messages, err := loadReportMessages(reportType)
	if err != nil {
		return fmt.Errorf("failed to load report messages: %w", err)
	}

	// Выбор случайного сообщения
	if len(messages) == 0 {
		return errors.New("no messages available for report")
	}
	index := rand.IntN(len(messages))

	// Определение причины жалобы
	reason, err := getReportReason(reportType)
	if err != nil {
		return err
	}

	// Отправка репорта
	_, err = api.AccountReportPeer(ctx, &tg.AccountReportPeerRequest{
		Peer:    peer,
		Reason:  reason,
		Message: messages[index],
	})
	return err
}

func getReportReason(reportType string) (tg.ReportReasonClass, error) {
	switch reportType {
	case "spam":
		return &tg.InputReportReasonSpam{}, nil
	case "author":
		return &tg.InputReportReasonCopyright{}, nil
	case "geo":
		return &tg.InputReportReasonGeoIrrelevant{}, nil
	case "drug":
		return &tg.InputReportReasonIllegalDrugs{}, nil
	case "child":
		return &tg.InputReportReasonChildAbuse{}, nil
	case "personal":
		return &tg.InputReportReasonPersonalDetails{}, nil
	case "porno":
		return &tg.InputReportReasonPornography{}, nil
	case "violence":
		return &tg.InputReportReasonViolence{}, nil
	default:
		return nil, fmt.Errorf("unknown report type: %s", reportType)
	}
}

func loadReportMessages(reportType string) ([]string, error) {
	msgPath := filepath.Join("messages", reportType+".json")
	jsonData, err := os.ReadFile(msgPath)
	if err != nil {
		return nil, fmt.Errorf("error reading messages file: %w", err)
	}

	if len(jsonData) == 0 {
		return nil, errors.New("messages file is empty")
	}

	var messageData MessageData
	if err := json.Unmarshal(jsonData, &messageData); err != nil {
		return nil, fmt.Errorf("JSON parsing error: %w", err)
	}

	if len(messageData.Messages) == 0 {
		return nil, errors.New("no messages available")
	}
	return messageData.Messages, nil
}

func moveToTrash(path string) {
	if _, err := os.Stat(trashPath); os.IsNotExist(err) {
		os.Mkdir(trashPath, 0755)
	}

	dest := filepath.Join(trashPath, filepath.Base(path))
	if err := os.Rename(path, dest); err != nil {
		log.Printf("Error moving to trash: %v", err)
	}
}
