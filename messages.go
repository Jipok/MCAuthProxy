package main

import (
	"fmt"
	"strings"
)

type Message struct {
	ru string
	en string
}

type MessageKey int

const (
	MsgRegistrationSuccess MessageKey = iota
	MsgSelectNickToDelete
	MsgNicknameDeleted
	MsgDeleteError
	MsgBadNickame
	MsgRequestSentToAdmin
	MsgAdminAckApprove
	MsgApproved
	MsgCantApprove
	MsgRegistrationTip
	MsgNicknameBusy
	MsgEmptyNicknameList
	MsgListCmd
	MsgDeleteCmd
	MsgOnlineCmd
)

///////////////////////////////////////////////////////////////////////////////

var messages = map[MessageKey]Message{
	MsgEmptyNicknameList: {
		ru: `📝 У вас пока нет зарегистрированных никнеймов
			Чтобы создать новый, просто отправьте желаемый никнейм сообщением`,
		en: `📝 You don't have any registered nicknames yet
			To create one, simply send your desired nickname in a message.`,
	},
	MsgBadNickame: {
		ru: `📝 Требования к никнейму:
			• Длина: от 3 до 16 символов
			• Разрешены: буквы (A-Z, a-z), цифры (0-9) и подчёркивание (_)`,
		en: `📝 Nickname requirements:
			• Length: 3 to 16 characters
			• Allowed: letters (A-Z, a-z), numbers (0-9), and underscore (_)`,
	},
	MsgSelectNickToDelete: {
		ru: `⚠️ Важное предупреждение:
			Удаление никнейма только освобождает его для регистрации другими игроками. Все данные персонажа на сервере (инвентарь, постройки, прогресс) останутся без изменений.

			Если кто-то позже зарегистрирует этот никнейм, он получит доступ к вашему персонажу на сервере.

			🗑️ Выберите никнейм для удаления:`,
		en: `⚠️ Important warning:
			Deleting a nickname only makes it available for registration by other players. All character data on the server (inventory, buildings, progress) will remain unchanged.

			If someone later registers this nickname, they will get access to your character on the server.
			
			🗑️ Select nickname to delete:`,
	},
	MsgNicknameDeleted: {
		ru: `✅ Никнейм освобождён`,
		en: `✅ Nickname has been released`,
	},
	MsgDeleteError: {
		ru: `⚠️ Невозможно удалить этот никнейм
			Вы можете удалять только те никнеймы, которые зарегистрировали сами.`,
		en: `⚠️ Cannot delete this nickname
			You can only delete nicknames that you have registered yourself.`,
	},
	MsgRequestSentToAdmin: {
		ru: `⏳ Ваша заявка на регистрацию отправлена администратору. 
			Пожалуйста, ожидайте одобрения.`,
		en: `⏳ Your registration request has been sent to the administrator.
			Please wait for approval.`,
	},
	MsgAdminAckApprove: {
		ru: `👤 Для регистрации нажмите:
			/a%d`,
		en: `👤 To approve registration click:
			/a%d`,
	},
	MsgApproved: {
		ru: `✅ Ваша заявка одобрена!
			Отправьте сообщение с желаемым никнеймом.`,
		en: `✅ Your request has been approved!
			Please send a message with your desired nickname.`,
	},
	MsgCantApprove: {
		ru: `⚠️ Не удалось связаться с пользователем
			Вероятно, пользователь заблокировал бота или удалил чат. Заявка отклонена.`,
		en: `⚠️ Failed to contact user
			The user has likely blocked the bot or deleted the chat. Request rejected.`,
	},
	MsgRegistrationTip: {
		ru: `💡 Вы можете зарегистрировать дополнительные никнеймы в любое время, просто отправив их в этот чат.`,
		en: `💡 You can register additional nicknames at any time by simply sending them in this chat.`,
	},
	MsgRegistrationSuccess: {
		ru: `✅ Регистрация успешна!

			📝 Как подключиться к серверу:
			1. Запустите Minecraft
			2. Выберите "Сетевая игра" → "Добавить сервер"
			3. В поле "Адрес сервера" введите:
			   ` + "`%s`" + `

			⚠️ Важно: этот адрес — ваш личный ключ доступа к серверу. Не передавайте его другим игрокам, иначе они смогут играть от вашего имени.

			❓ Возникли проблемы? Напишите %s`,
		en: `✅ Registration successful!

			📝 How to connect to the server:
			1. Launch Minecraft
			2. Select "Multiplayer" → "Add Server"
			3. In the "Server Address" field, enter:
			   ` + "`%s`" + `

			⚠️ Important: this address is your personal server access key. Do not share it with other players, as they will be able to play under your name.

			❓ Having problems? Contact %s`,
	},
	MsgNicknameBusy: {
		ru: `❌ Никнейм уже занят другим игроком
			Пожалуйста, выберите другой никнейм.`,
		en: `❌ This nickname is already taken by another player
			Please choose a different nickname.`,
	},
	//
	MsgListCmd: {
		ru: `📋 Показать ваши никнеймы и адреса для подключения`,
		en: `📋 Show your nicknames and connection addresses`,
	},
	MsgDeleteCmd: {
		ru: `🗑️ Удалить один из ваших никнеймов`,
		en: `🗑️ Delete one of your nicknames`,
	},
	MsgOnlineCmd: {
		ru: `👥 [ADMIN] Автообновляемый список игроков онлайн`,
		en: `👥 [ADMIN] Auto-updating online players list`,
	},
}

///////////////////////////////////////////////////////////////////////////////

func stripIndent(s string) string {
	lines := strings.Split(s, "\n")
	if len(lines) <= 1 {
		return s
	}
	// Skip first line
	for i := 1; i < len(lines); i++ {
		if len(lines[i]) > 0 {
			// Убираем 3 таба в начале
			lines[i] = strings.Replace(lines[i], "\t\t\t", "", 1)
		}
	}
	return strings.Join(lines, "\n")
}

func Msg(key MessageKey, args ...interface{}) string {
	msg := ""
	switch cfg.Lang {
	case "ru":
		msg = messages[key].ru
	default:
		msg = messages[key].en
	}
	msg = strings.TrimSpace(msg)
	msg = stripIndent(msg)
	return fmt.Sprintf(msg, args...)
}
