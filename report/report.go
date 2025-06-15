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

var trashPath = filepath.Join("trash")

var spamReportsMessages = []string{
	"Пользователь — шлюхобот, автоматически рассылает предложения интимных услуг за деньги, требую блокировки",
	"Аккаунт управляется роботом и предлагает сексуальные услуги с обманом по предоплате",
	"Спам от шлюхобота с подозрительными ссылками на сайты интимного характера, не поддавайтесь",
	"Пользователь использует чужие фотографии для рекламы интимных услуг, это мошенничество",
	"Шлюхобот угрожает публикацией личных данных при отказе от перевода денег, срочно разберитесь",
	"Мне пришло сообщение от бота с предложением встречи за деньги, нарушающее правила платформы",
	"Аккаунт массово рассылает спам с обещаниями интима в обмен на оплату подписки, заблокируйте",
	"Шлюхобот предлагает услуги секс-работников через поддельные профили, это нелегально",
	"Спам с угрозами расправой из-за неоплаты услуг, отправленный ботом, требуется модерация",
	"Пользователь-робот рекламирует сайты с интимными услугами, нарушая политику безопасности",
	"Мне прислали ссылку от шлюхобота на платформу с мошенническими интим-видеозвонками",
	"Аккаунт использует фишинговые ссылки для кражи данных под предлогом интимных услуг",
	"Шлюхобот предлагает «бесплатный секс» при переходе по подозрительным ссылкам, не верьте",
	"Спам от бота с обещаниями встреч с несовершеннолетними, это уголовное преступление",
	"Пользователь-робот массово отправляет угрозы интимного характера при отказе от общения",
	"Мне пришло сообщение от шлюхобота с требованием перевести деньги за «неполученный секс»",
	"Аккаунт рекламирует интимные услуги с поддельными отзывами, обманывает пользователей",
	"Спам с предложениями группового секса за деньги, отправленный шлюхоботом, удалите",
	"Шлюхобот угрожает публикацией фото из переписки при отказе от участия в его схемах",
	"Пользователь-робот предлагает интимные услуги с требованием предоплаты, это мошенничество",
	"Мне прислали ссылку от бота на сайт с интимными услугами, содержащий вредоносный код",
	"Аккаунт массово рассылает спам с обещаниями «секса без обязательств» через поддельные профили",
	"Шлюхобот использует чужие данные для создания фейковых анкет интимного характера",
	"Спам с угрозами судебного иска из-за якобы интимных отношений, отправленный ботом",
	"Пользователь-робот предлагает услуги секс-работников с требованием перевода денег без гарантий",
	"Мне пришло сообщение от шлюхобота с предложением «быстрого заработка через интим»",
	"Аккаунт рекламирует интимные услуги с обманчивыми условиями, нарушая правила платформы",
	"Спам с предложениями интимного характера от бота, использующего фото знаменитостей",
	"Шлюхобот угрожает физической расправой при отказе от встречи, вызовите полицию",
	"Пользователь-робот массово отправляет ссылки на сайты с интимным контентом и фишингом",
	"Мне прислали сообщение от бота с обещанием интима в обмен на подписку на мошеннический сайт",
	"Аккаунт использует поддельные истории о бедственном положении для интимного обмана",
	"Спам с угрозами публикации переписки из-за интимных предложений, отправленный шлюхоботом",
	"Шлюхобот предлагает интимные услуги с требованием перевода денег на анонимные кошельки",
	"Пользователь-робот массово рассылает спам с обещаниями «секса без последствий»",
	"Мне пришло сообщение от бота с предложением интима через поддельный профиль актрисы",
	"Аккаунт рекламирует интимные услуги с требованием предоплаты и скрывает реальные условия",
	"Спам от шлюхобота с подозрительными ссылками на сайты с интимным контентом и вирусами",
	"Пользователь-робот угрожает публикацией переписки из-за интимных предложений, не поддавайтесь",
	"Шлюхобот массово отправляет сообщения с обещаниями интима за денежное вознаграждение"}

func IsValid(username string) bool {
	re := regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9_]{4,31}$`)
	return re.MatchString(username)
}
func StartReport(filepath string, username string, reportType string) error {
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
		index := rand.IntN(len(spamReportsMessages))

		_, errReport := api.AccountReportPeer(actionCtx, &tg.AccountReportPeerRequest{
			Peer:    targetPeer,
			Reason:  &tg.InputReportReasonSpam{},
			Message: spamReportsMessages[index],
		})
		if errReport != nil {
			return errReport
		}
		return nil
	}); err != nil {
		os.Rename(filepath, trashPath)
		println(err.Error())
		return err
	}
	return nil
}
