package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/chaindead/telegram-mcp/internal/tg"
	"github.com/invopop/jsonschema"

	mcp "github.com/metoro-io/mcp-golang"
	"github.com/metoro-io/mcp-golang/transport/stdio"
	"github.com/rs/zerolog/log"
	"github.com/urfave/cli/v3"
)

func serve(ctx context.Context, cmd *cli.Command) error {
	appID := cmd.Int("app-id")
	appHash := cmd.String("api-hash")
	sessionPath := cmd.String("session")
	dryRun := cmd.Bool("dry")

	if schemaURL := cmd.String("schema-version"); schemaURL != "" {
		jsonschema.Version = schemaURL
	}

	_, err := os.Stat(sessionPath)
	if err != nil {
		return fmt.Errorf("session file not found(%s): %w", sessionPath, err)
	}

	server := mcp.NewServer(stdio.NewStdioServerTransport())
	client := tg.New(int(appID), appHash, sessionPath)

	if dryRun {
		answer, err := client.GetMe(tg.EmptyArguments{})
		if err != nil {
			return fmt.Errorf("get user: %w", err)
		}

		data, err := json.MarshalIndent(answer, "", "  ")
		if err != nil {
			return fmt.Errorf("marshal: %w", err)
		}

		log.Info().RawJSON("answer", data).Msg("Check GetMe: OK")

		answer, err = client.GetDialogs(tg.DialogsArguments{Offset: "", OnlyUnread: true})
		if err != nil {
			return fmt.Errorf("get dialogs: %w", err)
		}

		log.Info().RawJSON("answer", []byte(answer.Content[0].TextContent.Text)).Msg("Check GetDialogs: OK")

		answer, err = client.GetHistory(tg.HistoryArguments{Name: os.Getenv("TG_TEST_USERNAME")})
		if err != nil {
			return fmt.Errorf("get nickname history: %w", err)
		}

		answer, err = client.GetHistory(tg.HistoryArguments{Name: "cht[4626931529]"})
		if err != nil {
			return fmt.Errorf("get chat history: %w", err)
		}

		answer, err = client.GetHistory(tg.HistoryArguments{Name: "chn[2225853048:8934705438195741763]"})
		if err != nil {
			return fmt.Errorf("get chan history: %w", err)
		}

		log.Info().RawJSON("answer", []byte(answer.Content[0].TextContent.Text)).Msg("Check GetHistory: OK")

		answer, err = client.SendMessage(tg.SendArguments{Name: os.Getenv("TG_TEST_USERNAME"), Text: "test message"})
		if err != nil {
			log.Err(err).Msg("Check SendMessage: FAIL")
		} else {
			log.Info().RawJSON("answer", []byte(answer.Content[0].TextContent.Text)).Msg("Check SendMessage: OK")
		}

		answer, err = client.ReadHistory(tg.ReadArguments{Name: os.Getenv("TG_TEST_USERNAME")})
		if err != nil {
			log.Err(err).Msg("Check ReadHistory: FAIL")
		} else {
			log.Info().RawJSON("answer", []byte(answer.Content[0].TextContent.Text)).Msg("Check ReadHistory: OK")
		}

		return nil
	}

	err = server.RegisterTool("tg_me", "Get current telegram account info", client.GetMe)
	if err != nil {
		return fmt.Errorf("register tool: %w", err)
	}

	err = server.RegisterTool("tg_dialogs", "Get list of telegram dialogs (chats, channels, users)", client.GetDialogs)
	if err != nil {
		return fmt.Errorf("register dialogs tool: %w", err)
	}

	err = server.RegisterTool("tg_dialog", "Get messages of telegram dialog", client.GetHistory)
	if err != nil {
		return fmt.Errorf("register dialogs tool: %w", err)
	}

	err = server.RegisterTool("tg_send", "Send message to dialog", client.SendMessage)
	if err != nil {
		return fmt.Errorf("register send tool: %w", err)
	}

	err = server.RegisterTool("tg_read", "Mark dialog messages as read", client.ReadHistory)
	if err != nil {
		return fmt.Errorf("register read tool: %w", err)
	}

	err = server.RegisterTool("tg_search_groups", "Search for public Telegram groups and channels by keyword", client.SearchGroups)
	if err != nil {
		return fmt.Errorf("register search groups tool: %w", err)
	}

	err = server.RegisterTool("tg_join_group", "Join a Telegram group or channel by username or invite link", client.JoinGroup)
	if err != nil {
		return fmt.Errorf("register join group tool: %w", err)
	}

	err = server.RegisterTool("tg_leave_group", "Leave a Telegram group or channel", client.LeaveGroup)
	if err != nil {
		return fmt.Errorf("register leave group tool: %w", err)
	}

	err = server.RegisterTool("tg_search_messages", "Search for messages containing a keyword in a specific Telegram dialog, group, or channel", client.SearchMessages)
	if err != nil {
		return fmt.Errorf("register search messages tool: %w", err)
	}

	if err := server.Serve(); err != nil {
		return fmt.Errorf("serve: %w", err)
	}

	<-ctx.Done()

	return nil
}
