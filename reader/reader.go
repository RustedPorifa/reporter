package reader

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gotd/td/session"
	"github.com/gotd/td/session/tdesktop"
	"github.com/gotd/td/telegram"
	"golang.org/x/net/proxy"
)

var checkURL = "https://ifconfig.me/ip"

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
		os.Remove(tdataPath)
		return nil
	}); err != nil {
		println(err.Error())
	}
}

func GetReports() (int, error) {
	entry, err := os.ReadDir("sessions")
	if err != nil {
		return 0, err
	}
	return len(entry), nil
}

func DeleteTrash() {
	entry, _ := os.ReadDir("trash")
	for _, file := range entry {
		pathToFile := filepath.Join("trash", file.Name())
		if file.IsDir() {
			os.RemoveAll(pathToFile)
		} else {
			os.Remove(pathToFile)
		}
	}
}

func GetTrash() (string, error) {
	entry, err := os.ReadDir("trash")
	if err != nil {
		return "err", err
	}

	return strconv.Itoa(len(entry)), nil
}

func TestProxy(NotParsedUrl string) (bool, string) {
	arrayOfProxy := strings.Split(NotParsedUrl, ":")
	proxyAdress := arrayOfProxy[0] + ":" + arrayOfProxy[1]
	username := arrayOfProxy[2]
	passwd := arrayOfProxy[3]
	auth := proxy.Auth{
		User:     username,
		Password: passwd,
	}
	dialer, err := proxy.SOCKS5("tcp", proxyAdress, &auth,
		&net.Dialer{
			Timeout: 10 * time.Second,
		},
	)
	if err != nil {
		return false, err.Error()
	}
	tr := &http.Transport{
		Dial:                  dialer.Dial,
		ResponseHeaderTimeout: 15 * time.Second,
	}

	myClient := &http.Client{
		Transport: tr,
		Timeout:   30 * time.Second,
	}
	resp, err := myClient.Get(checkURL)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error making request:", err)
		return false, err.Error()
	}
	defer resp.Body.Close()
	return true, "None"
}
