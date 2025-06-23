package emulator

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math/rand"
	"os"
	"path/filepath"
	"sync"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/gotd/td/session"
	"github.com/gotd/td/telegram"
	"github.com/gotd/td/tg"
)

var (
	emojis  = []string{"❤️‍🔥", "❤️", "🥰", "🏆"}
	emojiMu sync.Mutex
)

func humanDelay(minSec, maxSec int) {
	r := rand.Intn(maxSec-minSec) + minSec
	time.Sleep(time.Duration(r) * time.Second)
}

func buildSession(sessionPath string, ctx context.Context) (*telegram.Client, error) {
	dataBytes, err := os.ReadFile(sessionPath)
	if err != nil {
		fmt.Println("Error reading session file:", err)
		return nil, err
	}

	var data *session.Data
	if err := json.Unmarshal(dataBytes, &data); err != nil {
		fmt.Println("Error unmarshaling session:", err)
		return nil, err
	}

	// Создание хранилища и загрузка данных сессии
	storage := &session.StorageMemory{}
	loader := session.Loader{Storage: storage}
	if err := loader.Save(ctx, data); err != nil {
		fmt.Println("Error loading session into storage:", err)
		return nil, err
	}
	client := telegram.NewClient(telegram.TestAppID, telegram.TestAppHash, telegram.Options{
		SessionStorage:      storage,
		ReconnectionBackoff: nil,
		NoUpdates:           true,
		MaxRetries:          2,
	})
	return client, nil

}

func JoinChannelWithRotation(channelUsername string, bot *tgbotapi.BotAPI, chatToSend int64) {

	var accountsPool []string
	entries, err := os.ReadDir("sessions")
	if err != nil {
		log.Printf("Error reading sessions directory: %v", err)
		return
	}

	for _, file := range entries {

		if !file.IsDir() {
			accountsPool = append(accountsPool, filepath.Join("sessions", file.Name()))
		}
	}

	log.Printf("Total accounts: %d", len(accountsPool))

	for len(accountsPool) > 0 {
		batchSize := rand.Intn(7) + 4
		if batchSize > len(accountsPool) {
			batchSize = len(accountsPool)
		}

		rand.Shuffle(len(accountsPool), func(i, j int) {
			accountsPool[i], accountsPool[j] = accountsPool[j], accountsPool[i]
		})
		batch := accountsPool[:batchSize]

		log.Printf("Processing batch of %d accounts", batchSize)

		var wg sync.WaitGroup
		for _, path := range batch {
			wg.Add(1)
			go func(sessionPath string) {
				defer func() {
					if r := recover(); r != nil {
						log.Printf("Panic in account %s: %v", sessionPath, r)
					}
					wg.Done()
				}()
				log.Printf("Processing account: %s", sessionPath)
				ProcessAndJoin(sessionPath, channelUsername)
			}(path)
		}
		wg.Wait()
		accountsPool = accountsPool[batchSize:]

		if len(accountsPool) > 0 {
			jitter := time.Duration(rand.Intn(6) - 3)
			pause := 20*time.Minute + jitter*time.Minute
			log.Printf("Next batch in %v (%d accounts left)", pause, len(accountsPool))
			time.Sleep(pause)
		}
	}

	msg := tgbotapi.NewMessage(chatToSend, "Накрутка завершена. Канал: "+channelUsername)
	if _, err := bot.Send(msg); err != nil {
		log.Printf("Error sending completion message: %v", err)
	} else {
		log.Printf("Completion message sent")
	}
}

func ProcessAndJoin(path string, channel string) {
	ctx := context.Background()
	client, err := buildSession(path, ctx)
	if err != nil {
		log.Println("NEW ERROR WHHILE BUILDING", err)
		return
	}

	actionCtx, cancel := context.WithTimeout(ctx, time.Duration(rand.Intn(10)+4)*time.Minute)
	defer cancel()

	if err := client.Run(actionCtx, func(ctx context.Context) error {
		JoinChannel(actionCtx, client.API(), channel)
		return nil
	}); err != nil {
		log.Println("NEW ERORR WHILE LOAD SESSION: ", err)
	}
}

func JoinChannel(ctx context.Context, api *tg.Client, channelUsername string) error {
	const max_retries = 5
	for attempt := 0; attempt < max_retries; attempt++ {
		// Разрешаем юзернейм в объект канала
		res, err := api.ContactsResolveUsername(ctx, &tg.ContactsResolveUsernameRequest{
			Username: channelUsername,
		})
		if err != nil {
			fmt.Printf("Resolve username attempt %d: %v\n", attempt+1, err)
			humanDelay(2, 5)
			continue
		}

		// Ищем целевой канал по юзернейму
		var targetChannel *tg.Channel
		for _, chat := range res.Chats {
			if channel, ok := chat.(*tg.Channel); ok && channel.Username == channelUsername {
				targetChannel = channel
				break
			}
		}

		if targetChannel == nil {
			return errors.New("channel not found in resolved entities")
		}

		// Присоединяемся к каналу если не участник
		if targetChannel.Left {
			inputChannel := &tg.InputChannel{
				ChannelID:  targetChannel.ID,
				AccessHash: targetChannel.AccessHash,
			}

			if _, err := api.ChannelsJoinChannel(ctx, inputChannel); err != nil {
				fmt.Printf("Join channel attempt %d: %v\n", attempt+1, err)
				humanDelay(3, 7)
				continue
			}
			fmt.Printf("✅ Joined channel: %s\n", targetChannel.Title)
			time.Sleep(3 * time.Second) // Даем серверу обработать вступление
			readChannel(ctx, api, channelUsername)
		}

		return nil
	}
	return errors.New("max retries exceeded")
}

func readChannel(ctx context.Context, api *tg.Client, channelUsername string) error {
	const maxRetries = 3
	for attempt := 0; attempt < maxRetries; attempt++ {
		// Разрешаем юзернейм в объект канала
		res, err := api.ContactsResolveUsername(ctx, &tg.ContactsResolveUsernameRequest{
			Username: channelUsername,
		})
		if err != nil {
			log.Printf("Resolve username attempt %d: %v", attempt+1, err)
			humanDelay(2, 5)
			continue
		}

		// Ищем целевой канал по юзернейму
		var targetChannel *tg.Channel
		for _, chat := range res.Chats {
			if channel, ok := chat.(*tg.Channel); ok && channel.Username == channelUsername {
				targetChannel = channel
				break
			}
		}

		if targetChannel == nil {
			return errors.New("channel not found in resolved entities")
		}

		// Пропускаем если не участник
		if targetChannel.Left {
			return errors.New("account is not a member of the channel")
		}

		inputChannel := &tg.InputChannel{
			ChannelID:  targetChannel.ID,
			AccessHash: targetChannel.AccessHash,
		}

		peer := &tg.InputPeerChannel{
			ChannelID:  targetChannel.ID,
			AccessHash: targetChannel.AccessHash,
		}

		// Получаем последние сообщения (новые -> старые)
		history, err := api.MessagesGetHistory(ctx, &tg.MessagesGetHistoryRequest{
			Peer:  peer,
			Limit: 10,
		})
		if err != nil {
			log.Printf("Get history error: %v", err)
			humanDelay(3, 7)
			continue
		}

		// Извлекаем сообщения
		var messages []tg.MessageClass
		switch h := history.(type) {
		case *tg.MessagesMessages:
			messages = h.Messages
		case *tg.MessagesMessagesSlice:
			messages = h.Messages
		case *tg.MessagesChannelMessages:
			messages = h.Messages
		default:
			log.Printf("Unsupported history type: %T", h)
			continue
		}

		if len(messages) == 0 {
			log.Println("No messages to read")
			return nil
		}

		// Берем ID самого нового сообщения (первое в списке)
		maxID := messages[0].GetID()

		// Отмечаем прочитанным ДО реакций
		if _, err := api.ChannelsReadHistory(ctx, &tg.ChannelsReadHistoryRequest{
			Channel: inputChannel,
			MaxID:   maxID,
		}); err != nil {
			log.Printf("Read history error: %v", err)
		} else {
			log.Printf("📖 Marked messages as read up to %d", maxID)
		}

		// Ставим реакции на случайные сообщения
		for i := 0; i < min(3, len(messages)); i++ { // Макс 3 реакции
			humanDelay(1, 3)

			// Выбираем случайное сообщение
			msg := messages[rand.Intn(len(messages))]

			emojiMu.Lock()
			randomEmoji := emojis[rand.Intn(len(emojis))]
			emojiMu.Unlock()

			if _, err := api.MessagesSendReaction(ctx, &tg.MessagesSendReactionRequest{
				Peer:     peer,
				MsgID:    msg.GetID(),
				Reaction: []tg.ReactionClass{&tg.ReactionEmoji{Emoticon: randomEmoji}},
			}); err != nil {
				log.Printf("Reaction error: %v", err)
			} else {
				log.Printf("%s Reacted to message %d", randomEmoji, msg.GetID())
			}
		}

		return nil
	}
	return errors.New("max retries exceeded")
}

// Вспомогательная функция для минимума
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

/*func readChats(ctx context.Context, api *tg.Client) error {
	req := tg.MessagesGetDialogsRequest{
		Limit:      6,
		OffsetPeer: &tg.InputPeerEmpty{},
	}
	dialogs, err := api.MessagesGetDialogs(ctx, &req)
	if err != nil {
		println(err.Error())
	}
	var chats []tg.ChatClass
	switch d := dialogs.(type) {
	case *tg.MessagesDialogs:
		chats = d.Chats
	case *tg.MessagesDialogsSlice:
		chats = d.Chats
	default:
		return errors.New("unsupported dialog type")
	}

	// 3. Перемешивание чатов в случайном порядке
	rand.Shuffle(len(chats), func(i, j int) {
		chats[i], chats[j] = chats[j], chats[i]
	})
	for _, chat := range chats {
		// Случайная задержка между чатами (5-30 секунд)
		humanDelay(5, 30)

		switch c := chat.(type) {
		case *tg.Channel:
			// 5. Пропуск некоторых каналов (10% вероятности)
			if rand.Float32() < 0.1 {
				continue
			}

			// 6. Случайное количество сообщений (1-5)
			messagesLimit := rand.Intn(5) + 1

			// 7. Создаем peer напрямую (без ResolveUsername)
			peer := &tg.InputPeerChannel{
				ChannelID:  c.ID,
				AccessHash: c.AccessHash,
			}

			// 8. Получение истории сообщений
			history, err := api.MessagesGetHistory(ctx, &tg.MessagesGetHistoryRequest{
				Peer:  peer,
				Limit: messagesLimit,
			})
			if err != nil {
				log.Printf("History error in %s: %v", c.Title, err)
				continue
			}

			// 9. Обработка истории (упрощенная)
			var messages []tg.MessageClass
			switch h := history.(type) {
			case *tg.MessagesMessages:
				messages = h.Messages
			case *tg.MessagesMessagesSlice:
				messages = h.Messages
			case *tg.MessagesChannelMessages:
				messages = h.Messages
			default:
				continue
			}

			// 10. Отметка прочтения только если есть сообщения
			if len(messages) > 0 {
				maxID := messages[len(messages)-1].GetID()
				if _, err := api.ChannelsReadHistory(ctx, &tg.ChannelsReadHistoryRequest{
					Channel: &tg.InputChannel{
						ChannelID:  c.ID,
						AccessHash: c.AccessHash,
					},
					MaxID: maxID,
				}); err != nil {
					log.Printf("Read error in %s: %v", c.Title, err)
				}
			}

			// rand.Float32() < 0.3 &&
			if len(messages) > 0 {
				// Выбираем случайное сообщение
				msg := messages[rand.Intn(len(messages))]

				// Случайная задержка перед реакцией
				humanDelay(1, 5)

				// Случайный эмодзи
				emojiMu.Lock()
				emoji := emojis[rand.Intn(len(emojis))]
				emojiMu.Unlock()

				if _, err := api.MessagesSendReaction(ctx, &tg.MessagesSendReactionRequest{
					Peer:     peer,
					MsgID:    msg.GetID(),
					Reaction: []tg.ReactionClass{&tg.ReactionEmoji{Emoticon: emoji}},
				}); err != nil {
					log.Printf("Reaction error: %v", err)
				}
			}
		}
	}
	return nil
}

func processAccount(path string) {
	ctx := context.Background()
	client, err := buildSession(path, ctx)
	if err != nil {
		log.Println("NEW ERROR WHHILE BUILDING", err)
		return
	}

	actionCtx, cancel := context.WithTimeout(ctx, time.Duration(rand.Intn(4))*time.Minute)
	defer cancel() // Гарантированно выполнится при выходе из processAccount

	// Синхронная обработка с контролем контекста
	if err := client.Run(actionCtx, func(ctx context.Context) error {
		errRead := readChats(ctx, client.API()) // Должна учитывать ctx!
		if errRead != nil {
			log.Println("NEW ERROR OCCURED", errRead)
		}
		return nil
	}); err != nil {
		log.Println("NEW ERORR WHILE LOAD SESSION: ", err)
	}
}


*/
