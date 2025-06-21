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
func EmulateActivity() {
	var accountsPool = []string{}
	entry, _ := os.ReadDir("LoadedSessions")
	for _, file := range entry {
		accountsPool = append(accountsPool, filepath.Join("LoadedSessions", file.Name()))
	}
	for {
		rand.Shuffle(len(accountsPool), func(i, j int) {
			accountsPool[i], accountsPool[j] = accountsPool[j], accountsPool[i]
		})

		selected := accountsPool[:rand.Intn(len(accountsPool))+1] // +1 чтобы избежать 0
		var wg sync.WaitGroup

		for _, path := range selected {
			wg.Add(1)
			go func(p string) {
				defer wg.Done()
				processAccount(p)
			}(path)
		}
		wg.Wait() // Ждем завершения всех горутин перед следующей итерацией
		time.Sleep(time.Duration(rand.Intn(30)) * time.Minute)
	}
}
func JoinChannelWithRotation(channelUsername string, bot *tgbotapi.BotAPI, chatToSend int64) {
	var accountsPool []string
	entries, _ := os.ReadDir("sessions")
	for _, file := range entries {
		accountsPool = append(accountsPool, filepath.Join("sessions", file.Name()))
	}

	for len(accountsPool) > 0 {
		batchSize := rand.Intn(7) + 4
		if batchSize > len(accountsPool) {
			batchSize = len(accountsPool)
		}

		// 4. Выбор случайного пакета аккаунтов
		rand.Shuffle(len(accountsPool), func(i, j int) {
			accountsPool[i], accountsPool[j] = accountsPool[j], accountsPool[i]
		})
		batch := accountsPool[:batchSize]

		// 5. Параллельная обработка пакета
		var wg sync.WaitGroup
		for _, path := range batch {
			wg.Add(1)
			go func(sessionPath string) {
				defer wg.Done()
				ProcessAndJoin(sessionPath, channelUsername)
			}(path)
		}
		wg.Wait()

		// 6. Удаление обработанных аккаунтов из пула
		accountsPool = accountsPool[batchSize:]

		// 7. Пауза перед следующим пакетом (если остались аккаунты)
		if len(accountsPool) > 0 {
			jitter := time.Duration(rand.Intn(6) - 3)
			pause := 20*time.Minute + jitter*time.Minute
			time.Sleep(pause)
		}
	}
	msg := tgbotapi.NewMessage(chatToSend, "Накрутка в канал окончена. Канал: "+channelUsername)
	if _, err := bot.Send(msg); err != nil {
		log.Printf("Error sending message: %v", err)
	}
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
func ProcessAndJoin(path string, channel string) {
	ctx := context.Background()
	client, err := buildSession(path, ctx)
	if err != nil {
		log.Println("NEW ERROR WHHILE BUILDING", err)
		return
	}

	actionCtx, cancel := context.WithTimeout(ctx, time.Duration(rand.Intn(4))*time.Minute)
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
	const max_retries = 3
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

		// Создаем InputChannel для работы с API
		inputChannel := &tg.InputChannel{
			ChannelID:  targetChannel.ID,
			AccessHash: targetChannel.AccessHash,
		}

		// Создаем InputPeerChannel для получения истории
		peer := &tg.InputPeerChannel{
			ChannelID:  targetChannel.ID,
			AccessHash: targetChannel.AccessHash,
		}

		// Получаем последние сообщения
		history, err := api.MessagesGetHistory(ctx, &tg.MessagesGetHistoryRequest{
			Peer:  peer,
			Limit: 10, // Увеличили лимит для примера
		})
		if err != nil {
			fmt.Printf("Get history error: %v\n", err)
			return fmt.Errorf("get history: %w", err)
		}

		// Извлекаем сообщения из разных типов ответа
		var messages []tg.MessageClass
		switch h := history.(type) {
		case *tg.MessagesMessages:
			messages = h.Messages
		case *tg.MessagesMessagesSlice:
			messages = h.Messages
		case *tg.MessagesChannelMessages:
			messages = h.Messages
		default:
			fmt.Printf("Unhandled history type: %T\n", h)
			return errors.New("unexpected history type")
		}

		// Проверяем, есть ли сообщения
		if len(messages) == 0 {
			fmt.Println("No messages to read")
			return nil
		}

		// Отмечаем сообщения как прочитанные
		var maxMsgID int
		for _, msg := range messages {
			if msg.GetID() > maxMsgID {
				maxMsgID = msg.GetID()
			}
		}

		// Используем ChannelsReadHistory с InputChannel
		if _, err := api.ChannelsReadHistory(ctx, &tg.ChannelsReadHistoryRequest{
			Channel: inputChannel,
			MaxID:   maxMsgID,
		}); err != nil {
			fmt.Printf("Read history error: %v\n", err)
		} else {
			fmt.Printf("📖 Marked %d messages as read\n", len(messages))
		}

		// Опционально: ставим реакции на последние сообщения
		for i, msg := range messages {
			humanDelay(1, 2)

			emojiMu.Lock()
			randomEmoji := emojis[rand.Intn(len(emojis))]
			emojiMu.Unlock()

			if _, err := api.MessagesSendReaction(ctx, &tg.MessagesSendReactionRequest{
				Peer:     peer,
				MsgID:    msg.GetID(),
				Reaction: []tg.ReactionClass{&tg.ReactionEmoji{Emoticon: randomEmoji}},
			}); err != nil {
				fmt.Printf("Reaction %d error: %v\n", i+1, err)
			} else {
				fmt.Printf("%s Reacted to message %d\n", randomEmoji, msg.GetID())
			}
			humanDelay(1, 2)
		}

		return nil
	}
	return errors.New("max retries exceeded")
}

func readChats(ctx context.Context, api *tg.Client) error {
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
