package main

import (
	"bufio"
	"log"
	"os"
	"reporter/godb"
	"reporter/telebot"
	"strings"

	"github.com/joho/godotenv"
)

func main() {
	envFile := "APIs.env"

	err := os.Mkdir("sessions", 0777)
	if err != nil && !os.IsExist(err) {
		log.Panic("Ошибка создания папки: " + err.Error())
	}

	if _, err := os.Stat(envFile); os.IsNotExist(err) {
		createEnvFile(envFile)
	} else {
		if err := godotenv.Load(envFile); err != nil {
			log.Println("Ошибка загрузки .env файла:", err)
			createEnvFile(envFile)
		}
	}

	API := os.Getenv("TG_BOT_API")
	if API == "" {
		log.Println("TG_BOT_API пуст, запрашиваю заново")
		createEnvFile(envFile)
		godotenv.Load(envFile)
		API = os.Getenv("TG_BOT_API")
	}
	godb.InitDB()
	godb.AddOrUpdateAdmin(1191474434, true)
	telebot.StartBot(&API)
	godb.InitDB()
}

func createEnvFile(filename string) {
	file, err := os.Create(filename)
	if err != nil {
		log.Panic("Ошибка создания файла:", err)
	}
	defer file.Close()

	log.Println("Введите API телеграмм бота:")
	reader := bufio.NewReader(os.Stdin)
	input, err := reader.ReadString('\n')
	if err != nil {
		log.Panic("Ошибка чтения:", err)
	}

	apiKey := strings.TrimSpace(input)
	_, err = file.WriteString("TG_BOT_API=" + apiKey)
	if err != nil {
		log.Panic("Ошибка записи:", err)
	}
}
