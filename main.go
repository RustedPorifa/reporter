package main

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"reporter/godb"
	"reporter/telebot"
	"strconv"
	"strings"

	"github.com/joho/godotenv"
)

func main() {
	envFile := "APIs.env"

	err := os.Mkdir("sessions", 0777)
	if err != nil && !os.IsExist(err) {
		log.Panic("Ошибка создания папки: " + err.Error())
	}
	errTrash := os.Mkdir("trash", 0777)
	if errTrash != nil && !os.IsExist(errTrash) {
		log.Panic("Ошибка создания папки: " + errTrash.Error())
	}

	errTdata := os.Mkdir("tdata_sessions", 0777)
	if errTdata != nil && !os.IsExist(errTdata) {
		log.Panic("Ошибка создания папки: " + errTdata.Error())
	}

	errMsg := os.Mkdir("tdata_sessions", 0777)
	if errMsg != nil && !os.IsExist(errMsg) {
		log.Panic("Ошибка создания папки: " + errMsg.Error())
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
	admines, err := godb.GetAllAdmins()
	if err != nil {
		log.Println(err)
	}
	if len(admines) < 1 {
		log.Println("Не обнаружено ни одного администратора! Бот не ответит вам, если вы не администратор")
		addAdministrator()
	}
	telebot.StartBot(&API)

}

func addAdministrator() {
	reader := bufio.NewReader(os.Stdin)

	for {
		fmt.Print("Введите Telegram ID пользователя для добавления в администраторы: ")
		input, err := reader.ReadString('\n')
		if err != nil {
			fmt.Println("Ошибка чтения:", err)
			continue
		}

		input = strings.TrimSpace(input)
		if input == "" {
			fmt.Println("Ошибка: пустой ввод")
			continue
		}
		//ХУЙ
		userID, err := strconv.ParseInt(input, 10, 64)
		if err != nil {
			fmt.Println("Ошибка: введенный id не является числом")
			continue
		}

		if userID <= 0 {
			fmt.Println("Ошибка: ID должен быть положительным числом")
			continue
		}

		// Проверка существующих прав
		isAdmin, err := godb.IsAdmin(userID)
		if err == nil && isAdmin {
			fmt.Printf("Пользователь %d уже является администратором\n", userID)
			return
		}

		// Подтверждение
		fmt.Printf("Добавить пользователя %d как администратора? (y/n): ", userID)
		confirm, _ := reader.ReadString('\n')
		confirm = strings.TrimSpace(strings.ToLower(confirm))

		if confirm != "y" && confirm != "yes" {
			fmt.Println("Действие отменено")
			continue
		}

		// Добавление в БД
		if err := godb.AddOrUpdateAdmin(userID, true); err != nil {
			fmt.Println("Ошибка добавления администратора:", err)
			continue
		}

		fmt.Printf("✅ Пользователь %d успешно добавлен как администратор\n", userID)
		return
	}
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
