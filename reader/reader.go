package reader

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/gotd/td/session"
	"github.com/gotd/td/session/tdesktop"
	"github.com/gotd/td/telegram"
)

func LoadSessions(fileName string) {

	ctx := context.Background()

	tdataPath := filepath.Join("tdata_sessions", fileName, "tdata")

	accounts, err := tdesktop.Read(tdataPath, nil)
	if err != nil {
		fmt.Println("Skipping account:", err)
		return
	}
	println(tdataPath, accounts)
	if len(accounts) == 0 {
		fmt.Println("Empty account, skipping")
		return
	}

	data, err := session.TDesktopSession(accounts[0])
	if err != nil {
		fmt.Println("Skipping account (session error):", err)
		return
	}

	// Сохранение сессии
	storage := new(session.StorageMemory)
	loader := session.Loader{Storage: storage}
	if err := loader.Save(ctx, data); err != nil {
		fmt.Println("Skipping account (save error):", err)
		return
	}
	sessionFilename := filepath.Join("sessions", fileName+".session")
	sessionData, err := json.Marshal(data)
	if err != nil {
		fmt.Println("Error marshaling session:", err)
	}
	if err := os.WriteFile(sessionFilename, sessionData, 0600); err != nil {
		fmt.Println("Error writing session to file:", err)
		return
	}
	os.Remove(tdataPath)
	// Запуск клиента
	client := telegram.NewClient(telegram.TestAppID, telegram.TestAppHash, telegram.Options{
		SessionStorage: storage,
	})
	if err := client.Run(ctx, func(ctx context.Context) error {
		return nil
	}); err != nil {
		println(err.Error())
	}
	return
}

func GetReports() (int, error) {
	entry, err := os.ReadDir("sessions")
	if err != nil {
		return 0, err
	}
	return len(entry), nil
}
