package telebot

import (
	"archive/zip"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"reporter/emulator"
	"reporter/godb"
	"reporter/reader"
	"reporter/report"
	"strconv"
	"strings"
	"sync"

	tgb "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// Keyboards
var StartKeyboard = tgb.NewInlineKeyboardMarkup(
	tgb.NewInlineKeyboardRow(
		tgb.NewInlineKeyboardButtonData("Администрация", "admin-keyboard"),
	),
	tgb.NewInlineKeyboardRow(
		tgb.NewInlineKeyboardButtonData("Аккаунты", "accs-keyboard"),
	),
	tgb.NewInlineKeyboardRow(
		tgb.NewInlineKeyboardButtonData("Прокси", "proxy-keyboard"),
	),
	tgb.NewInlineKeyboardRow(
		tgb.NewInlineKeyboardButtonData("Накрутка", "recruiment-start"),
	),
	tgb.NewInlineKeyboardRow(
		tgb.NewInlineKeyboardButtonData("Мусорка", "trash"),
	),
	tgb.NewInlineKeyboardRow(
		tgb.NewInlineKeyboardButtonData("Начать снос", "report-start"),
	),
)

var adminKeyboard = tgb.NewInlineKeyboardMarkup(
	tgb.NewInlineKeyboardRow(
		tgb.NewInlineKeyboardButtonData("Добавить администратора", "add-admin"),
	),
	tgb.NewInlineKeyboardRow(
		tgb.NewInlineKeyboardButtonData("Все администраторы", "show-admins"),
	),
	tgb.NewInlineKeyboardRow(
		tgb.NewInlineKeyboardButtonData("Меню", "menu"),
	),
)

var accKeyboard = tgb.NewInlineKeyboardMarkup(
	tgb.NewInlineKeyboardRow(
		tgb.NewInlineKeyboardButtonData("Кол-во макс. жалоб", "max-reports"),
	),
	tgb.NewInlineKeyboardRow(
		tgb.NewInlineKeyboardButtonData("Загрузить аккаунты (.zip)", "download-accs"),
	),
	tgb.NewInlineKeyboardRow(
		tgb.NewInlineKeyboardButtonData("Меню", "menu"),
	),
)

var proxyKeyboard = tgb.NewInlineKeyboardMarkup(
	tgb.NewInlineKeyboardRow(
		tgb.NewInlineKeyboardButtonData("Всего прокси", "max-proxy"),
	),
	tgb.NewInlineKeyboardRow(
		tgb.NewInlineKeyboardButtonData("Загрузить прокси", "download-proxy"),
	),
	tgb.NewInlineKeyboardRow(
		tgb.NewInlineKeyboardButtonData("Меню", "menu"),
	),
)

var reportKeyboard = tgb.NewInlineKeyboardMarkup(
	tgb.NewInlineKeyboardRow(
		tgb.NewInlineKeyboardButtonData("Спам", "report-complaint-spam"),
		tgb.NewInlineKeyboardButtonData("Авторское право", "report-complaint-author"),
	),
	tgb.NewInlineKeyboardRow(
		tgb.NewInlineKeyboardButtonData("Раскрытые гео", "report-complaint-geo"),
		tgb.NewInlineKeyboardButtonData("Наркотики", "report-complaint-drug"),
	),
	tgb.NewInlineKeyboardRow(
		tgb.NewInlineKeyboardButtonData("Жестокость к детям", "report-complaint-child"),
		tgb.NewInlineKeyboardButtonData("Личные данные", "report-complaint-personal"),
	),
	tgb.NewInlineKeyboardRow(
		tgb.NewInlineKeyboardButtonData("Порнография", "report-complaint-porno"),
		tgb.NewInlineKeyboardButtonData("Насилие", "report-complaint-violence"),
	),
)

//Keyboard end

// Vars
var (
	UserState   = make(map[int64]string)
	UserStateMu sync.Mutex
)

var (
	ToReport   = make(map[int64]string)
	ToReportMu sync.Mutex
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
			SendMessage(bot, msg)
		}
	} else if isAdmin {
		UserStateMu.Lock()
		state := UserState[Message.From.ID]
		UserStateMu.Unlock()
		switch state {
		case "add_admin":
			userID, err := strconv.ParseInt(Message.Text, 10, 64)
			if err != nil {
				SendMessage(bot, tgb.NewMessage(Message.Chat.ID, "Введен неверный формат id, попробуйте ещё раз"))
				return
			}
			godb.AddOrUpdateAdmin(userID, true)
			SendMessage(bot, tgb.NewMessage(Message.Chat.ID, "Новый администратор успешно добавлен"))
		case "wait_for_username":
			if report.IsValid(Message.Text) {
				msg := tgb.NewMessage(Message.Chat.ID, "Выберите вид жалобы для сноса\nДа, вид жалобы может повлиять на вид санкции со стороны телеграмма")
				msg.ReplyMarkup = reportKeyboard
				SendMessage(bot, msg)
				ToReportMu.Lock()
				ToReport[Message.From.ID] = Message.Text
				ToReportMu.Unlock()

			} else {
				SendMessage(bot, tgb.NewMessage(Message.Chat.ID, "Юзернейм невалидный, напишите только символы без @"))
			}

		case "wait_for_zip":
			handleZipUpload(bot, Message)
		case "wait_for_channel":
			re := regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9_]{4,31}$`)
			if !re.MatchString(Message.Text) {
				SendMessage(bot, tgb.NewMessage(Message.Chat.ID, "Имя канал невалидное, попробуйте без ссылки"))
				go emulator.JoinChannelWithRotation(Message.Text, bot, Message.Chat.ID)
			} else {
				SendMessage(bot, tgb.NewMessage(Message.Chat.ID, "Накрутка на адаптер началась, в течении дня она завершится"))
				go emulator.JoinChannelWithRotation(Message.Text, bot, Message.Chat.ID)
			}
		case "wait_for_proxy":
			to_check := Message.Text
			re := regexp.MustCompile(`^((25[0-5]|2[0-4]\d|1\d{2}|[1-9]?\d)\.){3}(25[0-5]|2[0-4]\d|1\d{2}|[1-9]?\d):(\d{1,5}):([a-zA-Z0-9]+):([a-zA-Z0-9]+)$`)
			if !re.MatchString(to_check) {
				SendMessage(bot, tgb.NewMessage(Message.Chat.ID, "Отправленный прокси неверен, попробуйте снова. Загрузите прокси ввиде:\nip:порт:логин:пароль"))
			} else {
				SendMessage(bot, tgb.NewMessage(Message.Chat.ID, "Отправленный прокси прошел проверку регуляркой, ожидайте тест запроса..."))
				isValid, typicalError := reader.TestProxy(to_check)
				if isValid {
					godb.AddProxy(to_check, 0)
					SendMessage(bot, tgb.NewMessage(Message.Chat.ID, "Прокси успешно прошёл проверку и был добавлен!"))
				} else {
					SendMessage(bot, tgb.NewMessage(Message.Chat.ID, "Прокси не прошёл проверку запросом. Ошибка: "+typicalError))
				}
			}
		}

	}

}

func handleCallback(bot *tgb.BotAPI, callback *tgb.CallbackQuery) {
	defer func() {
		if _, err := bot.Request(tgb.NewCallback(callback.ID, "")); err != nil {
			log.Printf("Callback answer error: %v", err)
		}
	}()
	switch callback.Data {
	case "trash":
		trashAmout, _ := reader.GetTrash()
		SendMessage(bot, tgb.NewMessage(callback.Message.Chat.ID, "Очистка мусора началась, всего элементов: "+trashAmout))
		go reader.DeleteTrash()

	//Keyboards
	case "menu":
		msg := tgb.NewEditMessageText(callback.Message.Chat.ID, callback.Message.MessageID, "Добро пожаловать в бота!\nПожалуйста, выберите опцию ниже")
		msg.ReplyMarkup = &StartKeyboard
		editMessage(bot, msg)
	case "admin-keyboard":
		edit := tgb.NewEditMessageText(callback.Message.Chat.ID, callback.Message.MessageID, "Вы в панеле администрации, выберите опцию ниже.")
		edit.ReplyMarkup = &adminKeyboard
		editMessage(bot, edit)
	case "accs-keyboard":
		edit := tgb.NewEditMessageText(callback.Message.Chat.ID, callback.Message.MessageID, "Вы в панеле управления аккаунтами для массовых жалоб, выберите опцию ниже.")
		edit.ReplyMarkup = &accKeyboard
		editMessage(bot, edit)

	case "proxy-keyboard":
		msg := tgb.NewEditMessageText(callback.Message.Chat.ID, callback.Message.MessageID, "Выберите опцию насчет прокси ниже")
		msg.ReplyMarkup = &proxyKeyboard
		editMessage(bot, msg)
	//Administration
	case "add-admin":
		SendMessage(bot, tgb.NewMessage(callback.Message.Chat.ID, "Отправьте ID администратора"))
		UserStateMu.Lock()
		UserState[callback.From.ID] = "add_admin"
		UserStateMu.Unlock()
	case "show-admins":
		admins, err := godb.GetAllAdmins()
		if err != nil {
			SendMessage(bot, tgb.NewMessage(callback.Message.Chat.ID, err.Error()))
			return
		}

		var adminsBuilder strings.Builder
		adminsBuilder.WriteString("ID администраторов:\n")

		for i, adminID := range admins {
			adminsBuilder.WriteString(fmt.Sprintf("%d. %d\n", i, adminID))
		}

		msg := tgb.NewMessage(callback.Message.Chat.ID, adminsBuilder.String())
		SendMessage(bot, msg)
		return
	//Accounts
	case "max-reports":
		reportsCount, err := reader.GetReports()
		if err != nil {
			SendMessage(bot, tgb.NewMessage(callback.Message.Chat.ID, err.Error()))
			return
		}
		msg := ("%s Максимальное количество репортов, готовых для отправки (если сессии ещё живы): " + strconv.Itoa(reportsCount))
		SendMessage(bot, tgb.NewMessage(callback.Message.Chat.ID, msg))
		return
	case "download-accs":
		UserStateMu.Lock()
		UserState[callback.From.ID] = "wait_for_zip"
		UserStateMu.Unlock()
		SendMessage(bot, tgb.NewMessage(callback.Message.Chat.ID, "Отправьте .zip файл, который состоит из Tdata для загрузки"))
		//reports
	case "report-start":
		SendMessage(bot, tgb.NewMessage(callback.Message.Chat.ID, "Введите юзернейм пользователя для сноса\nВид юзернейма должен представлять исключительно сам юзернейм\n@dunduk -> dunduk"))
		UserStateMu.Lock()
		UserState[callback.From.ID] = "wait_for_username"
		UserStateMu.Unlock()
	case "recruiment-start":
		SendMessage(bot, tgb.NewMessage(callback.Message.Chat.ID, "Введите канал пользователя для сноса\nВид юзернейма должен представлять исключительно сам канал\nt.me//dunduk -> dunduk"))
		UserStateMu.Lock()
		UserState[callback.From.ID] = "wait_for_channel"
		UserStateMu.Unlock()
	//Proxy
	case "max-proxy":
		proxy_count, _ := godb.GetProxyCount()
		SendMessage(bot, tgb.NewMessage(callback.Message.Chat.ID, "Всего прокси: "+proxy_count))
	case "download-proxy":
		SendMessage(bot, tgb.NewMessage(callback.Message.Chat.ID, "Загрузите прокси ввиде:\nip:порт:логин:пароль\nПрокси является socks5. Купить можно на сайте: https://getproxy.io"))
		UserStateMu.Lock()
		UserState[callback.From.ID] = "wait_for_proxy"
		UserStateMu.Unlock()
	default:
		parts := strings.Split(callback.Data, "-")
		ToReportMu.Lock()
		usernameReport := ToReport[callback.From.ID]
		ToReportMu.Unlock()
		if len(parts) == 3 && parts[1] == "complaint" && usernameReport != "" {
			SendMessage(bot, tgb.NewMessage(callback.Message.Chat.ID, "Запрос успешно обработан! Жалобы будут отправлены в скором времени."))
			entry, err := os.ReadDir("sessions")
			if err != nil {
				SendMessage(bot, tgb.NewMessage(callback.Message.Chat.ID, err.Error()))
				return
			}
			for _, sess := range entry {
				err := report.StartReport(filepath.Join("sessions", sess.Name()), usernameReport, parts[2], sess.Name())
				if err != nil {
					println(err.Error())
					continue
				}
			}
			SendMessage(bot, tgb.NewMessage(callback.Message.Chat.ID, "Жалобы успешно отправлены, запрос обработан"))
		}
	}
}

func SendMessage(bot *tgb.BotAPI, msg tgb.MessageConfig) {
	if _, err := bot.Send(msg); err != nil {
		log.Printf("Error sending message: %v", err)
	}
}

func editMessage(bot *tgb.BotAPI, edit tgb.EditMessageTextConfig) {
	if _, err := bot.Send(edit); err != nil {
		log.Printf("Error editing message: %v", err)
	}
}

func handleZipUpload(bot *tgb.BotAPI, msg *tgb.Message) {
	go func() {
		file, err := bot.GetFile(tgb.FileConfig{FileID: msg.Document.FileID})
		if err != nil {
			SendMessage(bot, tgb.NewMessage(msg.Chat.ID, err.Error()))
			return
		}

		url := fmt.Sprintf("https://api.telegram.org/file/bot%s/%s", bot.Token, file.FilePath)
		resp, err := http.Get(url)
		if err != nil {
			log.Println("Ошибка скачивания файла:", err)
			return
		}
		defer resp.Body.Close()

		out, err := os.Create(msg.Document.FileName)
		if err != nil {
			log.Println("Ошибка создания файла:", err)
			return
		}
		defer out.Close()

		if _, err = io.Copy(out, resp.Body); err != nil {
			log.Println("Ошибка сохранения файла:", err)
			return
		}

		if err := unzip(out.Name(), "tdata_sessions"); err != nil {
			log.Printf("Ошибка распаковки: %v", err)
			SendMessage(bot, tgb.NewMessage(msg.Chat.ID, "Ошибка обработки файла"))
			return
		}

		log.Printf("Файл %s успешно обработан", msg.Document.FileName)
		SendMessage(bot, tgb.NewMessage(msg.Chat.ID, "Файл успешно обработан, начинаю загрзку сессий, ожидайте..."))

		UserStateMu.Lock()
		delete(UserState, msg.From.ID)
		UserStateMu.Unlock()
		entry, _ := os.ReadDir("tdata_sessions")
		for _, session := range entry {
			go reader.LoadSessions(session.Name())
		}

	}()
}

func unzip(src string, dest string) error {
	r, err := zip.OpenReader(src)
	if err != nil {
		return err
	}
	defer r.Close()

	for _, f := range r.File {
		fpath := filepath.Join(dest, f.Name)
		if !strings.HasPrefix(fpath, filepath.Clean(dest)+string(os.PathSeparator)) {
			return fmt.Errorf("недопустимый путь файла: %s", fpath)
		}

		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(fpath, os.ModePerm); err != nil {
				return err
			}
			continue
		}

		// Создаем каталоги для файла, если необходимо
		if err := os.MkdirAll(filepath.Dir(fpath), os.ModePerm); err != nil {
			return err
		}

		rc, err := f.Open()
		if err != nil {
			return err
		}
		defer rc.Close()

		outFile, err := os.Create(fpath)
		if err != nil {
			return err
		}
		defer outFile.Close()

		if _, err := io.Copy(outFile, rc); err != nil {
			return err
		}
	}
	os.Remove(src)
	return nil
}
