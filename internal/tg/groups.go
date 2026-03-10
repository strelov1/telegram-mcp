package tg

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/gotd/td/tg"
	mcp "github.com/metoro-io/mcp-golang"
	"github.com/pkg/errors"
)

// --- tg_search_groups ---

type SearchGroupsArguments struct {
	Query string `json:"query" jsonschema:"required,description=Search keyword for groups and channels"`
	Limit int    `json:"limit,omitempty" jsonschema:"description=Maximum number of results (default 20, max 100)"`
}

type SearchGroupsResult struct {
	ID       int64  `json:"id"`
	Title    string `json:"title"`
	Username string `json:"username,omitempty"`
	Type     string `json:"type"`
	Members  int    `json:"members,omitempty"`
}

type SearchGroupsResponse struct {
	Results []SearchGroupsResult `json:"results"`
	Total   int                  `json:"total"`
}

func (c *Client) SearchGroups(args SearchGroupsArguments) (*mcp.ToolResponse, error) {
	limit := args.Limit
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}

	var response SearchGroupsResponse

	client := c.T()
	if err := client.Run(context.Background(), func(ctx context.Context) error {
		api := client.API()

		found, err := api.ContactsSearch(ctx, &tg.ContactsSearchRequest{
			Q:     args.Query,
			Limit: limit,
		})
		if err != nil {
			return fmt.Errorf("contacts search: %w", err)
		}

		var results []SearchGroupsResult
		for _, chat := range found.GetChats() {
			switch ch := chat.(type) {
			case *tg.Channel:
				if ch.Broadcast || ch.Megagroup {
					chatType := "channel"
					if ch.Megagroup {
						chatType = "supergroup"
					}
					username, _ := ch.GetUsername()
					result := SearchGroupsResult{
						ID:       ch.ID,
						Title:    ch.Title,
						Username: username,
						Type:     chatType,
					}
					if p, ok := ch.GetParticipantsCount(); ok {
						result.Members = p
					}
					results = append(results, result)
				}
			case *tg.Chat:
				results = append(results, SearchGroupsResult{
					ID:      ch.ID,
					Title:   ch.Title,
					Type:    "chat",
					Members: ch.ParticipantsCount,
				})
			}
		}

		if results == nil {
			results = []SearchGroupsResult{}
		}
		response = SearchGroupsResponse{Results: results, Total: len(results)}
		return nil
	}); err != nil {
		return nil, errors.Wrap(err, "search groups failed")
	}

	jsonData, err := json.Marshal(response)
	if err != nil {
		return nil, errors.Wrap(err, "failed to marshal response")
	}
	return mcp.NewToolResponse(mcp.NewTextContent(string(jsonData))), nil
}

// --- tg_join_group ---

type JoinGroupArguments struct {
	Target string `json:"target" jsonschema:"required,description=Username (e.g. @groupname) or invite link (t.me/+ or t.me/joinchat/)"`
}

type JoinGroupResponse struct {
	Success bool   `json:"success"`
	Title   string `json:"title,omitempty"`
	Type    string `json:"type,omitempty"`
	ID      int64  `json:"id,omitempty"`
	Message string `json:"message,omitempty"`
}

func extractInviteHash(target string) string {
	for _, prefix := range []string{
		"https://t.me/+", "https://t.me/joinchat/",
		"http://t.me/+", "http://t.me/joinchat/",
		"t.me/+", "t.me/joinchat/",
	} {
		if idx := strings.Index(target, prefix); idx != -1 {
			return target[idx+len(prefix):]
		}
	}
	return ""
}

func extractChatFromUpdates(updates tg.UpdatesClass) (title string, chatType string, id int64) {
	var chats []tg.ChatClass
	switch u := updates.(type) {
	case *tg.Updates:
		chats = u.GetChats()
	case *tg.UpdatesCombined:
		chats = u.GetChats()
	}
	for _, chat := range chats {
		switch ch := chat.(type) {
		case *tg.Channel:
			chatType = "channel"
			if ch.Megagroup {
				chatType = "supergroup"
			}
			return ch.Title, chatType, ch.ID
		case *tg.Chat:
			return ch.Title, "chat", ch.ID
		}
	}
	return "", "", 0
}

func (c *Client) JoinGroup(args JoinGroupArguments) (*mcp.ToolResponse, error) {
	var response JoinGroupResponse

	client := c.T()
	if err := client.Run(context.Background(), func(ctx context.Context) error {
		api := client.API()

		if hash := extractInviteHash(args.Target); hash != "" {
			updates, err := api.MessagesImportChatInvite(ctx, hash)
			if err != nil {
				if strings.Contains(err.Error(), "INVITE_REQUEST_SENT") {
					response = JoinGroupResponse{Success: true, Message: "Join request sent, waiting for admin approval"}
					return nil
				}
				return fmt.Errorf("import chat invite: %w", err)
			}
			title, chatType, id := extractChatFromUpdates(updates)
			response = JoinGroupResponse{Success: true, Title: title, Type: chatType, ID: id}
			return nil
		}

		username := strings.TrimPrefix(args.Target, "@")
		for _, prefix := range []string{"https://t.me/", "http://t.me/", "t.me/"} {
			username = strings.TrimPrefix(username, prefix)
		}

		resolved, err := api.ContactsResolveUsername(ctx, &tg.ContactsResolveUsernameRequest{
			Username: username,
		})
		if err != nil {
			return fmt.Errorf("resolve username %q: %w", username, err)
		}

		var inputChannel *tg.InputChannel
		for _, chat := range resolved.GetChats() {
			if ch, ok := chat.(*tg.Channel); ok {
				inputChannel = &tg.InputChannel{ChannelID: ch.ID, AccessHash: ch.AccessHash}
				break
			}
		}
		if inputChannel == nil {
			return fmt.Errorf("no channel found for username %q", username)
		}

		updates, err := api.ChannelsJoinChannel(ctx, inputChannel)
		if err != nil {
			if strings.Contains(err.Error(), "INVITE_REQUEST_SENT") {
				response = JoinGroupResponse{Success: true, Message: "Join request sent, waiting for admin approval"}
				return nil
			}
			return fmt.Errorf("join channel: %w", err)
		}
		title, chatType, id := extractChatFromUpdates(updates)
		response = JoinGroupResponse{Success: true, Title: title, Type: chatType, ID: id}
		return nil
	}); err != nil {
		return nil, errors.Wrap(err, "join group failed")
	}

	jsonData, err := json.Marshal(response)
	if err != nil {
		return nil, errors.Wrap(err, "failed to marshal response")
	}
	return mcp.NewToolResponse(mcp.NewTextContent(string(jsonData))), nil
}

// --- tg_leave_group ---

type LeaveGroupArguments struct {
	Name string `json:"name" jsonschema:"required,description=Username, channel ID as chn[id:hash], or chat ID as cht[id]"`
}

type LeaveGroupResponse struct {
	Success bool `json:"success"`
	Left    bool `json:"left"`
}

func (c *Client) LeaveGroup(args LeaveGroupArguments) (*mcp.ToolResponse, error) {
	var response LeaveGroupResponse

	client := c.T()
	if err := client.Run(context.Background(), func(ctx context.Context) error {
		api := client.API()

		inputPeer, err := getInputPeerFromName(ctx, api, args.Name)
		if err != nil {
			return fmt.Errorf("get inputPeer from name: %w", err)
		}

		switch peer := inputPeer.(type) {
		case *tg.InputPeerChannel:
			_, err = api.ChannelsLeaveChannel(ctx, &tg.InputChannel{
				ChannelID:  peer.ChannelID,
				AccessHash: peer.AccessHash,
			})
			if err != nil {
				return fmt.Errorf("leave channel: %w", err)
			}
		case *tg.InputPeerChat:
			_, err = api.MessagesDeleteChatUser(ctx, &tg.MessagesDeleteChatUserRequest{
				ChatID: peer.ChatID,
				UserId: &tg.InputUserSelf{},
			})
			if err != nil {
				return fmt.Errorf("leave chat: %w", err)
			}
		case *tg.InputPeerUser:
			return fmt.Errorf("cannot leave a personal dialog")
		default:
			return fmt.Errorf("unsupported peer type: %T", inputPeer)
		}

		response = LeaveGroupResponse{Success: true, Left: true}
		return nil
	}); err != nil {
		return nil, errors.Wrap(err, "leave group failed")
	}

	jsonData, err := json.Marshal(response)
	if err != nil {
		return nil, errors.Wrap(err, "failed to marshal response")
	}
	return mcp.NewToolResponse(mcp.NewTextContent(string(jsonData))), nil
}
