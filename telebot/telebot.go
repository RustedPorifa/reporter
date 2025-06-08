package telebot

import (
	"archive/zip"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"reporter/godb"
	"reporter/reader"
	"strconv"
	"strings"
	"sync"

	tgb "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

var StartKeyboard = tgb.NewInlineKeyboardMarkup(
	tgb.NewInlineKeyboardRow(
		tgb.NewInlineKeyboardButtonData("Загрузить аккаунты (.zip)", "upload-accs"),
		tgb.NewInlineKeyboardButtonData("Добавить администратора", "add-admin"),
	),
	tgb.NewInlineKeyboardRow(
		tgb.NewInlineKeyboardButtonData("Все администраторы", "show-admins"),
		tgb.NewInlineKeyboardButtonData("Кол-во жалоб", "max-reports"),
	),
	tgb.NewInlineKeyboardRow(
		tgb.NewInlineKeyboardButtonData("Начать снос", "report-start"),
	),
)

var (
	UserState   = make(map[int64]string)
	UserStateMu sync.Mutex
)

func StartBot(API *string) {
	bot, err := tgb.NewBotAPI(*API)
	if err != nil {
		log.Panic(err)
	}
	bot.Debug = false
	log.Printf("Authorized on account %s", bot.Self.UserName)
	u := tgb.NewUpdate(0)
	u.Timeout = 60

	updates := bot.GetUpdatesChan(u)

	for update := range updates {
		go func(u tgb.Update) {
			if u.Message != nil {
				handleMessage(bot, u.Message)
			} else if u.CallbackQuery != nil {
				handleCallback(bot, u.CallbackQuery)
			}
		}(update)
	}
}

func handleMessage(bot *tgb.BotAPI, Message *tgb.Message) {
	isAdmin, err := godb.IsAdmin(Message.From.ID)
	if err != nil {
		log.Panic(err)
	}

	if Message.IsCommand() && isAdmin {
		switch Message.Command() {
		case "start":
			msg := tgb.NewMessage(Message.Chat.ID, "Добро пожаловать в бота!\nПожалуйста, выберите опцию ниже")
			msg.ReplyMarkup = StartKeyboard
			sendMessage(bot, msg)
		}
	} else {
		UserStateMu.Lock()
		state := UserState[Message.From.ID]
		UserStateMu.Unlock()
		switch state {
		case "add_admin":
			userID, err := strconv.ParseInt(Message.Text, 10, 64)
			if err != nil {
				sendMessage(bot, tgb.NewMessage(Message.Chat.ID, "Введен неверный формат id, попробуйте ещё раз"))
				return
			}
			godb.AddOrUpdateAdmin(userID, true)
			sendMessage(bot, tgb.NewMessage(Message.Chat.ID, "Новый администратор успешно добавлен"))
		case "wait_for_username":
			sendMessage(bot, tgb.NewMessage(Message.Chat.ID, "Отправка массовых жалоб началась, ожидайте"))
			reader.CollectAccs(Message.Text)
		}
	}

}

func handleCallback(bot *tgb.BotAPI, callback *tgb.CallbackQuery) {
	switch callback.Data {
	case "add-admin":
		sendMessage(bot, tgb.NewMessage(callback.Message.Chat.ID, "Отправьте ID администратора"))
		UserStateMu.Lock()
		UserState[callback.From.ID] = "add_admin"
		UserStateMu.Unlock()
	case "show-admins":
		admins, err := godb.GetAllAdmins()
		if err != nil {
			sendMessage(bot, tgb.NewMessage(callback.Message.Chat.ID, err.Error()))
			return
		}

		var adminsBuilder strings.Builder
		adminsBuilder.WriteString("ID администраторов:\n")

		for i, adminID := range admins {
			adminsBuilder.WriteString(fmt.Sprintf("%d. %d\n", i, adminID))
		}

		msg := tgb.NewMessage(callback.Message.Chat.ID, adminsBuilder.String())
		sendMessage(bot, msg)
		return
	case "max-reports":
		reportsCount, err := reader.GetReports()
		if err != nil {
			sendMessage(bot, tgb.NewMessage(callback.Message.Chat.ID, err.Error()))
			return
		}
		msg := fmt.Sprintf("%s", "Максимальное количество репортов, готовых для отправки (если сессии ещё живы): "+strconv.Itoa(reportsCount))
		sendMessage(bot, tgb.NewMessage(callback.Message.Chat.ID, msg))
		return
	case "report-start":
		sendMessage(bot, tgb.NewMessage(callback.Message.Chat.ID, "Введите юзернейм пользователя для сноса"))
		UserStateMu.Lock()
		UserState[callback.Message.From.ID] = "wait_for_username"
		UserStateMu.Unlock()
	}
}

func sendMessage(bot *tgb.BotAPI, msg tgb.MessageConfig) {
	if _, err := bot.Send(msg); err != nil {
		log.Printf("Error sending message: %v", err)
	}
}

func editMessage(bot *tgb.BotAPI, edit tgb.EditMessageTextConfig) {
	if _, err := bot.Send(edit); err != nil {
		log.Printf("Error editing message: %v", err)
	}
}

func unzip(src string, dest string) error {
	r, err := zip.OpenReader(src)
	if err != nil {
		return err
	}
	defer r.Close()

	for _, f := range r.File {
		fpath := filepath.Join(dest, f.Name)

		// Защита от выхода за пределы dest (безопасность)
		if !strings.HasPrefix(fpath, filepath.Clean(dest)+string(os.PathSeparator)) {
			return fmt.Errorf("недопустимый путь файла: %s", fpath)
		}

		if f.FileInfo().IsDir() {
			// Создаем каталог
			if err := os.MkdirAll(fpath, os.ModePerm); err != nil {
				return err
			}
			continue
		}

		// Создаем каталоги для файла, если необходимо
		if err := os.MkdirAll(filepath.Dir(fpath), os.ModePerm); err != nil {
			return err
		}

		// Открываем файл внутри архива
		rc, err := f.Open()
		if err != nil {
			return err
		}
		defer rc.Close()

		// Создаем файл на диске
		outFile, err := os.Create(fpath)
		if err != nil {
			return err
		}
		defer outFile.Close()

		// Копируем содержимое файла
		if _, err := io.Copy(outFile, rc); err != nil {
			return err
		}
	}
	os.Remove(src)
	return nil
}
