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

func IsValid(username string) bool {
	re := regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9_]{4,31}$`)
	return re.MatchString(username)
}
func StartReport(pathToFile string, username string, reportType string, fileName string) error {
	// Проверка username перед началом операции
	if !IsValid(username) {
		return fmt.Errorf("invalid username format: %s", username)
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
		return performReport(ctx, client.API(), username, reportType)
	}); err != nil {
		log.Println(err)
		//moveToTrash(pathToFile)
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

func performReport(ctx context.Context, api *tg.Client, username, reportType string) error {
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

	// Загрузка сообщений для репорта
	messages, err := loadReportMessages(reportType)
	if err != nil {
		return errors.New("проверьте существует ли папка messages")
	}

	// Выбор случайного сообщения
	index := rand.IntN(len(messages))

	// Отправка репорта
	_, err = api.AccountReportPeer(ctx, &tg.AccountReportPeerRequest{
		Peer:    targetPeer,
		Reason:  &tg.InputReportReasonSpam{},
		Message: messages[index],
	})
	return err
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
	os.Rename(path, dest)
}
