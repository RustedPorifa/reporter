package telebot

import (
	"log"
	"reporter/godb"
	"sync"

	tgb "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

var StartKeyboard = tgb.NewInlineKeyboardMarkup(
	tgb.NewInlineKeyboardRow(
		tgb.NewInlineKeyboardButtonData("Загрузить аккаунты (.zip)", "upload-accs"),
		tgb.NewInlineKeyboardButtonData("Добавить администратора", "add-admin"),
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
	}

}

func handleCallback(bot *tgb.BotAPI, Message *tgb.CallbackQuery) {

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
