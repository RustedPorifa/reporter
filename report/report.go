package report

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand/v2"
	"os"
	"path/filepath"
	"regexp"
	"time"

	"github.com/gotd/td/session"
	"github.com/gotd/td/telegram"
	"github.com/gotd/td/tg"
)

type MessageData struct {
	Messages []string `json:"messages"`
}

var trashPath = filepath.Join("trash")

func IsValid(username string) bool {
	re := regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9_]{4,31}$`)
	return re.MatchString(username)
}
func StartReport(pathToFile string, username string, reportType string) error {
	ctx := context.Background()
	actionCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	dataBytes, err := os.ReadFile(pathToFile)
	if err != nil {
		fmt.Println("Error reading session file:", err)
		return err
	}

	var data *session.Data
	if err := json.Unmarshal(dataBytes, &data); err != nil {
		fmt.Println("Error unmarshaling session:", err)
		return err
	}

	storage := &session.StorageMemory{}
	loader := session.Loader{Storage: storage}
	if err := loader.Save(ctx, data); err != nil {
		fmt.Println("Error loading session into storage:", err)
		return err
	}
	client := telegram.NewClient(telegram.TestAppID, telegram.TestAppHash, telegram.Options{
		SessionStorage:      storage,
		ReconnectionBackoff: nil,
		NoUpdates:           true,
		MaxRetries:          2,
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
		var ReportsMessages MessageData
		jsonData, jserr := os.ReadFile(filepath.Join("messages", reportType+".json"))
		if jserr != nil {
			return jserr
		}
		err := json.Unmarshal([]byte(jsonData), &data)
		if err != nil {
			fmt.Println("Ошибка парсинга JSON:", err)
			return err
		}
		index := rand.IntN(len(ReportsMessages.Messages))

		_, errReport := api.AccountReportPeer(actionCtx, &tg.AccountReportPeerRequest{
			Peer:    targetPeer,
			Reason:  &tg.InputReportReasonSpam{},
			Message: ReportsMessages.Messages[index],
		})
		if errReport != nil {
			return errReport
		}
		return nil
	}); err != nil {
		os.Rename(pathToFile, trashPath)
		println(err.Error())
		return err
	}
	return nil
}
