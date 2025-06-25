package report

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand/v2"
	"net"
	"os"
	"path/filepath"
	"regexp"
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
	ctx := context.Background()
	actionCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()

	dataBytes, err := os.ReadFile(pathToFile)
	if err != nil {
		fmt.Println("Error reading session file:", err)
		return err
	}
	proxyURL := fileName
	var dialer proxy.Dialer
	if proxyURL != "" {
		parts := strings.Split(proxyURL, ":")
		if len(parts) < 4 {
			return errors.New("неверный формат прокси")
		}

		addr := net.JoinHostPort(parts[0], parts[1])
		auth := proxy.Auth{
			User:     parts[2],
			Password: parts[3],
		}

		var err error
		dialer, err = proxy.SOCKS5("tcp", addr, &auth, proxy.Direct)
		if err != nil {
			return fmt.Errorf("ошибка создания прокси: %w", err)
		}
	} else {
		dialer = proxy.Direct // Прямое подключение без прокси
	}

	var sessionInfo *session.Data
	if err := json.Unmarshal(dataBytes, &sessionInfo); err != nil {
		fmt.Println("Error unmarshaling session:", err)
		return err
	}

	storage := &session.StorageMemory{}
	loader := session.Loader{Storage: storage}
	if err := loader.Save(ctx, sessionInfo); err != nil {
		fmt.Println("Error loading session into storage:", err)
		return err
	}
	dialContext := func(ctx context.Context, network, addr string) (net.Conn, error) {
		return dialer.Dial(network, addr)
	}

	client := telegram.NewClient(telegram.TestAppID, telegram.TestAppHash, telegram.Options{
		SessionStorage:      storage,
		ReconnectionBackoff: nil,
		NoUpdates:           true,
		MaxRetries:          2,
		Resolver: dcs.Plain(dcs.PlainOptions{
			Dial: dialContext,
		}),
	})

	if err := client.Run(actionCtx, func(ctx context.Context) error {
		api := client.API()
		resolved, errResolved := api.ContactsResolveUsername(actionCtx, &tg.ContactsResolveUsernameRequest{
			Username: username,
		})
		if errResolved != nil {
			return errResolved
		}

		var targetPeer tg.InputPeerClass
		for _, user := range resolved.Users {
			if u, ok := user.(*tg.User); ok && u.Username == username {
				targetPeer = &tg.InputPeerUser{
					UserID:     u.ID,
					AccessHash: u.AccessHash,
				}
				break
			}
		}
		if targetPeer == nil {
			return errors.New("не было найдено ни единого пользователя")
		}

		msgPath := filepath.Join("messages", reportType+".json")
		jsonData, jserr := os.ReadFile(msgPath)
		if jserr != nil {
			return fmt.Errorf("error reading messages file: %w", jserr)
		}

		if len(jsonData) == 0 {
			return errors.New("messages file is empty")
		}

		var messageData MessageData
		if err := json.Unmarshal(jsonData, &messageData); err != nil {
			return fmt.Errorf("JSON parsing error: %w", err)
		}

		if len(messageData.Messages) == 0 {
			return errors.New("no messages available")
		}

		index := rand.IntN(len(messageData.Messages))

		_, errReport := api.AccountReportPeer(actionCtx, &tg.AccountReportPeerRequest{
			Peer:    targetPeer,
			Reason:  &tg.InputReportReasonSpam{},
			Message: messageData.Messages[index],
		})
		return errReport
	}); err != nil {
		if _, err := os.Stat(trashPath); !os.IsNotExist(err) {
			os.Rename(pathToFile, filepath.Join(trashPath, filepath.Base(pathToFile)))
		}
		return err
	}
	return nil
}
