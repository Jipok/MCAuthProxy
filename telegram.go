package main

import (
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"
	"github.com/PaulSonOfLars/gotgbot/v2/ext/handlers"
	"github.com/PaulSonOfLars/gotgbot/v2/ext/handlers/filters/message"
	mapset "github.com/deckarep/golang-set/v2"
)

var allowedIDs = mapset.NewSet[int64]()

func startTgBot() *ext.Updater {

	bot, err := gotgbot.NewBot(cfg.BotToken, &gotgbot.BotOpts{
		BotClient: &gotgbot.BaseBotClient{
			Client: http.Client{},
			DefaultRequestOpts: &gotgbot.RequestOpts{
				Timeout: gotgbot.DefaultTimeout,
			},
		},
	})
	if err != nil {
		log.Fatal(err)
	}

	_, err = bot.SetMyCommands([]gotgbot.BotCommand{
		{
			Command:     "list",
			Description: Msg(MsgListCmd),
		},
		{
			Command:     "delete",
			Description: Msg(MsgDeleteCmd),
		},
	}, nil)
	if err != nil {
		log.Fatal(err)
	}

	// Create updater and dispatcher.
	dispatcher := ext.NewDispatcher(&ext.DispatcherOpts{
		// If an error is returned by a handler, log it and continue going.
		Error: func(b *gotgbot.Bot, ctx *ext.Context, err error) ext.DispatcherAction {
			log.Println("an error occurred while handling update:", err.Error())
			return ext.DispatcherActionNoop
		},
		MaxRoutines: ext.DefaultMaxRoutines,
	})
	updater := ext.NewUpdater(dispatcher, nil)

	dispatcher.AddHandler(handlers.NewMessage(message.Text, defaultHandler))

	// Start receiving updates.
	err = updater.StartPolling(bot, &ext.PollingOpts{
		DropPendingUpdates: false,
		GetUpdatesOpts: &gotgbot.GetUpdatesOpts{
			Timeout: 9,
			RequestOpts: &gotgbot.RequestOpts{
				Timeout: time.Second * 10,
			},
		},
	})
	if err != nil {
		log.Fatal(err)
	}

	return updater
}

func defaultHandler(b *gotgbot.Bot, ctx *ext.Context) error {
	// Allow only direct chat, no groups
	if ctx.EffectiveChat.Id != ctx.EffectiveSender.Id() {
		return nil
	}
	userID := ctx.EffectiveSender.Id()

	// Is registered?
	if !allowedIDs.Contains(userID) {
		ctx.EffectiveMessage.Forward(b, cfg.AdminID, nil)
		msg := Msg(MsgAdminAckApprove, userID)
		b.SendMessage(cfg.AdminID, msg, nil)
		ctx.EffectiveMessage.Reply(b, Msg(MsgRequestSentToAdmin), nil)
		return nil
	}

	if cfg.AdminID == userID {
		newIDstr, IsAppendCommand := strings.CutPrefix(ctx.EffectiveMessage.Text, "/a")
		if IsAppendCommand {
			newID, err := strconv.ParseInt(newIDstr, 10, 64)
			if err != nil {
				_, err = ctx.EffectiveMessage.Reply(b, "Bad ID "+err.Error(), nil)
				return err
			}

			if !allowedIDs.Add(newID) {
				_, err = ctx.EffectiveMessage.Reply(b, "User already registered", nil)
				return err
			}

			_, err = b.SendMessage(newID, Msg(MsgApproved), nil)
			if err != nil {
				msg := Msg(MsgCantApprove) + "\n" + err.Error()
				_, err = ctx.EffectiveMessage.Reply(b, msg, nil)
				allowedIDs.Remove(newID)
				return err
			}
			_, err = ctx.EffectiveMessage.SetReaction(b, &gotgbot.SetMessageReactionOpts{
				Reaction: []gotgbot.ReactionType{gotgbot.ReactionTypeEmoji{Emoji: "👌"}},
			})
			return err
		}
	}

	// List
	if ctx.EffectiveMessage.Text == "/list" {
		msg := ""
		records, err := storage.FindByTgID(userID)
		if err != nil {
			_, err = ctx.EffectiveMessage.Reply(b, "Error, report admin pls. "+err.Error(), nil)
			return err
		}
		for _, record := range records {
			msg += fmt.Sprintf("`%s.%s`  %s\n", record.Token, cfg.BaseDomain, record.Nickname)
		}
		// Zero list?
		if len(records) == 0 {
			msg = Msg(MsgEmptyNicknameList)
		}
		_, err = ctx.EffectiveMessage.Reply(b, msg, &gotgbot.SendMessageOpts{ParseMode: "Markdown"})
		return err
	}

	// Delete list
	if ctx.EffectiveMessage.Text == "/delete" {
		msg := Msg(MsgSelectNickToDelete) + "\n"
		records, err := storage.FindByTgID(userID)
		if err != nil {
			_, err = ctx.EffectiveMessage.Reply(b, "Error, report admin pls. "+err.Error(), nil)
			return err
		}
		for _, record := range records {
			msg += fmt.Sprintf("/%s\n", record.Nickname)
		}
		// Zero list?
		if len(records) == 0 {
			msg = Msg(MsgEmptyNicknameList)
		}
		_, err = ctx.EffectiveMessage.Reply(b, msg, &gotgbot.SendMessageOpts{ParseMode: "Markdown"})
		return err
	}

	// Delete nickname
	nickToDelete, IsDeleteCommand := strings.CutPrefix(ctx.EffectiveMessage.Text, "/")
	if IsDeleteCommand {
		err := storage.DeleteByNickname(nickToDelete, userID)
		if err == ErrAccessDenied || err == ErrNicknameNotFound {
			_, err = ctx.EffectiveMessage.Reply(b, Msg(MsgDeleteError), nil)
			return err
		}
		if err != nil {
			_, err = ctx.EffectiveMessage.Reply(b, "Error. "+err.Error(), nil)
			return err
		}
		msg := fmt.Sprintf("User `%s` deleted nickname: %s\n", ctx.EffectiveUser.Username, nickToDelete)
		log.Print(msg)
		b.SendMessage(cfg.AdminID, msg, nil)
		_, err = ctx.EffectiveMessage.Reply(b, Msg(MsgNicknameDeleted), nil)
		return err
	}

	// New minecraft username
	mcUsername := ctx.EffectiveMessage.Text
	if !isValidMinecraftUsername(mcUsername) {
		_, err := ctx.EffectiveMessage.Reply(b, Msg(MsgBadNickame), nil)
		return err
	}

	tgname := ctx.EffectiveUser.Username + " " + ctx.EffectiveUser.FirstName + " " + ctx.EffectiveUser.LastName
	newUserInfo, err := storage.AddRecord(mcUsername, tgname, userID)
	if err == ErrNicknameExists {
		_, err = ctx.EffectiveMessage.Reply(b, Msg(MsgNicknameBusy), nil)
		return err
	}
	if err != nil {
		_, err = ctx.EffectiveMessage.Reply(b, err.Error(), nil)
		b.SendMessage(cfg.AdminID, err.Error(), nil)
		log.Printf("!!!!!!!!!!!!!!!!!!!\n %s", err.Error())
		return err
	}

	// Notify admin
	msg := fmt.Sprintf("User `%s` registered nickname: %s\n", newUserInfo.TgName, newUserInfo.Nickname)
	log.Print(msg)
	b.SendMessage(cfg.AdminID, msg, nil)

	address := newUserInfo.Token + "." + cfg.BaseDomain
	msg = Msg(MsgRegistrationSuccess, address, cfg.SupportName)
	_, err = ctx.EffectiveMessage.Reply(b, msg, &gotgbot.SendMessageOpts{ParseMode: "Markdown"})
	b.SendMessage(userID, Msg(MsgRegistrationTip), nil)
	return err
}