package report

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/gotd/td/session"
	"github.com/gotd/td/telegram"
	"github.com/gotd/td/tg"
)

var trashPath = filepath.Join("trash")

func StartReport(filepath string, username string) error {
	ctx := context.Background()
	actionCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	dataBytes, err := os.ReadFile(filepath)
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
		_, errReport := api.AccountReportPeer(actionCtx, &tg.AccountReportPeerRequest{
			Peer:    targetPeer,
			Reason:  &tg.InputReportReasonSpam{},
			Message: "Принудительно долгий спам сообщениям в целях угрозы / рекламы",
		})
		if errReport != nil {
			return errReport
		}
		return nil
	}); err != nil {
		os.Rename(filepath, trashPath)
		println(err.Error())
	}
	return nil
}
