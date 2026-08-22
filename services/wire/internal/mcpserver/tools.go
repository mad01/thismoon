package mcpserver

import (
	"context"
	"net/url"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/mad01/thismoon/services/wire/internal/client"
	"github.com/mad01/thismoon/services/wire/internal/ref"
)

// maxWaitSeconds mirrors the cap the server enforces on a blocking read. It is
// stated in the tool description so a session asks for a wait it will get.
const maxWaitSeconds = 120

// handlers carries the dependencies shared by all wire tools.
type handlers struct {
	client *client.Client
	webURL string // where the user watches a channel in a browser
	port   int    // the serve port, for minting connection strings
}

func registerTools(s *mcp.Server, h *handlers) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "wire_open",
		Description: "Open a channel so this session can talk to other sessions — two of them or ten. " +
			"Use it when work has to continue somewhere else — handing a task to another agent, or having a separate session exercise and report back on something you just changed — and you need to exchange messages rather than guess. " +
			"Pass a `name` describing the work (e.g. refactor-auth); omit it and one is generated. Names are lowercase letters, digits, dots, and dashes. " +
			"**Give the returned `connect` string to the other sessions verbatim** (e.g. wire://localhost:7432/refactor-auth). It is the entire join protocol: there are no invites or tokens, and it works as the `channel` argument of every other wire tool. " +
			"Show it to the user so they can paste it wherever the other sessions are. " +
			"`from` names you and puts you on the roster; every message you post must be signed the same way. Give yourself a real name — short and distinctive (planner, quill), never a generic placeholder like agent — because `to` addressing and the obligation ledger key on it. Everyone else arrives via wire_join. " +
			"Set `conventions` to the conversation's ground rules — they ride on the channel itself, so a session joining mid-conversation sees them without reading from the start. " +
			"A convention set that holds up: \"one question per message; answer with reply_to; address questions with to; reply_needed only when blocked\".",
	}, h.handleOpen)

	mcp.AddTool(s, &mcp.Tool{
		Name: "wire_join",
		Description: "Join a channel another session opened — call this FIRST, before posting or reading, whenever you were handed a connection string. " +
			"`from` is the name you are joining as; every message you post must be signed with it, and messages addressed `to` that name are yours to answer. " +
			"Pick the name yourself unless the briefing assigns you one: short and distinctive (quill, forge, mapper), never a generic placeholder like agent or assistant — addressing and the obligation ledger key on it, and two sessions both called agent are indistinguishable. " +
			"One call returns the full briefing: `channel.conventions` (the ground rules — follow them), `members` (who is on the channel), `cursor` (pass it to wire_read as since to read the backlog, or read from 0 for the full transcript), `awaiting_reply_by` (open obligations by addressee — check your name), and `awaiting_reply_off_roster` (obligation addressees nobody on the roster matches — check it for near-misses of your name; a misaddressed question will not appear under yours). " +
			"Your join lands in the transcript, so sessions blocked waiting for you wake immediately. " +
			"Set `note` to say what you are joining as or ready for — it becomes the join message's body. Joining a channel you are already on is a no-op that still returns the briefing.",
	}, h.handleJoin)

	mcp.AddTool(s, &mcp.Tool{
		Name: "wire_leave",
		Description: "Leave a channel: take yourself off the roster when your part is done but the conversation continues without you. " +
			"Other sessions see the leave in the transcript and stop addressing messages to you. " +
			"Set `note` to say why you are going and where your work landed. " +
			"Do not confuse this with wire_close — close ends the conversation for everyone; leave is just your exit.",
	}, h.handleLeave)

	mcp.AddTool(s, &mcp.Tool{
		Name: "wire_post",
		Description: "Post a message to a channel, addressed by its name, its id, or the connection string you were given. " +
			"`from` is REQUIRED — in a channel several sessions write to, an unsigned message cannot be answered. " +
			"Say what the readers need (what you did, what you need back, what you are blocked on); the transcript is the only context they get. " +
			"Use the protocol fields instead of prose sentinels: `kind` classifies intent — task, result, question, answer, ack, or note (note is for intros, status, and findings: substance that answers nothing and demands nothing). " +
			"`to` addresses the message to one agent by roster name — set it on every question and task when more than two agents share the channel, or the obligation belongs to nobody. " +
			"`reply_to` names the seq you are answering; set it on every response so an interleaved transcript stays followable. One reply_to per obligation: answer each question with its own message, and never bundle two questions into one seq — a single reply clears the whole seq from awaiting_reply whether or not it covered everything. " +
			"`reply_needed: true` says you are blocked until someone answers; it keeps the seq in awaiting_reply (and under the addressee's name in awaiting_reply_by) until a reply names it. It does not speed anything up — its value is that the debt survives in the record. " +
			"Returns the message's `seq`, which is the cursor the next reader resumes from and the id other messages reference in `reply_to`. Posting to a closed channel is an error.",
	}, h.handlePost)

	mcp.AddTool(s, &mcp.Tool{
		Name: "wire_read",
		Description: "Read the messages on a channel after a cursor, optionally waiting for the next one to arrive. " +
			"`channel` takes a name, an id, or a wire:// connection string — if another session handed you one, pass it straight through. " +
			"Pass `since` with the `cursor` from your last read to get only what is new; omit it to read the conversation from the start. " +
			"Set `wait` to a number of seconds (up to 120) to block until a message lands — that is how you wait for the other session's reply in one call instead of polling in a loop. " +
			"A wait that expires returns an empty list, not an error: read again, or give up. " +
			"Always keep the returned `cursor` for your next read, and check `channel.closed_at` — a closed channel will never produce another message, so stop waiting on it. " +
			"`awaiting_reply_by` maps roster names to the seqs each one owes an answer — **look up your own name and settle those before posting anything new**. `awaiting_reply` is every open obligation including unaddressed ones, which belong to whoever picks them up. " +
			"Do not trust `awaiting_reply_by` alone: `awaiting_reply_off_roster` lists the addressees on that map who are not on the roster, and a question misaddressed to you (a typo of your name, or sent after someone left) sits under a key you would never check. When it is non-empty, read the seqs under those names and judge which are yours — the server cannot tell a typo from a handoff to an agent that has not joined yet. " +
			"`members` is the current roster; `channel.conventions` carries the ground rules the opener declared — follow them.",
	}, h.handleRead)

	mcp.AddTool(s, &mcp.Tool{
		Name: "wire_list",
		Description: "List channels, most recently active first, with their message counts, participants, last line, and connection string. " +
			"Use it to find a conversation whose handle you have lost, or to see what other sessions are talking about. " +
			"Closed channels are left out unless `include_closed` is set.",
	}, h.handleList)

	mcp.AddTool(s, &mcp.Tool{
		Name: "wire_close",
		Description: "Close a channel when the conversation is finished. " +
			"Closing is terminal: no further messages, and every session waiting on the channel wakes immediately instead of blocking for a reply that will never come. " +
			"Give a `note` saying how it ended. The transcript stays readable afterwards.",
	}, h.handleClose)
}

// channelOut is the response shape for the channel-returning tools: the
// channel with its derived counts, plus the web URL for watching it live.
type channelOut struct {
	Channel client.Summary `json:"channel"`
	Connect string         `json:"connect" jsonschema:"the connection string to hand the other session; it works as the channel argument of every wire tool"`
	URL     string         `json:"url"     jsonschema:"web page where the user can watch this channel live"`
}

func (h *handlers) channel(s client.Summary) channelOut {
	return channelOut{Channel: s, Connect: s.Connect, URL: h.channelURL(s.Name)}
}

// channelURL is the human-facing page for one channel. The web page resolves
// a ref the same way the API does, so an id works as well as a name.
func (h *handlers) channelURL(ref string) string {
	return h.webURL + "/?channel=" + url.QueryEscape(ref)
}

// ── open ──

type openInput struct {
	Name        string `json:"name,omitempty"        jsonschema:"channel name to open, lowercase letters/digits/dots/dashes, e.g. refactor-auth; omit to have one generated"`
	Topic       string `json:"topic,omitempty"       jsonschema:"optional one-line description of what this conversation is for"`
	From        string `json:"from"                  jsonschema:"the name you give yourself for this conversation — short and distinctive (planner, quill), not a generic placeholder like agent; your messages are signed with it and replies are addressed to it"`
	Conventions string `json:"conventions,omitempty" jsonschema:"ground rules for the conversation (tag vocabulary, expected message shapes); carried on the channel so a late joiner sees them without reading from the start"`
}

func (h *handlers) handleOpen(
	ctx context.Context,
	_ *mcp.CallToolRequest,
	in openInput,
) (*mcp.CallToolResult, channelOut, error) {
	c, err := h.client.Open(ctx, client.OpenBody{
		Name:        in.Name,
		Topic:       in.Topic,
		From:        in.From,
		Conventions: in.Conventions,
	})
	if err != nil {
		return nil, channelOut{}, err
	}
	return nil, h.channel(c), nil
}

// ── join / leave ──

type joinInput struct {
	Channel string `json:"channel"        jsonschema:"channel name, id, or connection string to join"`
	From    string `json:"from"           jsonschema:"the name you join as — pick it yourself, short and distinctive (quill, forge), not a generic placeholder like agent; sign every later post with it, and answer messages addressed to it"`
	Note    string `json:"note,omitempty" jsonschema:"optional intro: what you are joining as or ready for; becomes the join message's body"`
}

func (h *handlers) handleJoin(
	ctx context.Context,
	_ *mcp.CallToolRequest,
	in joinInput,
) (*mcp.CallToolResult, channelOut, error) {
	c, err := h.client.Join(ctx, in.Channel, in.From, in.Note)
	if err != nil {
		return nil, channelOut{}, err
	}
	return nil, h.channel(c), nil
}

type leaveInput struct {
	Channel string `json:"channel"        jsonschema:"channel name, id, or connection string to leave"`
	From    string `json:"from"           jsonschema:"the name you joined as"`
	Note    string `json:"note,omitempty" jsonschema:"optional parting note: why you are going, where your work landed"`
}

func (h *handlers) handleLeave(
	ctx context.Context,
	_ *mcp.CallToolRequest,
	in leaveInput,
) (*mcp.CallToolResult, channelOut, error) {
	c, err := h.client.Leave(ctx, in.Channel, in.From, in.Note)
	if err != nil {
		return nil, channelOut{}, err
	}
	return nil, h.channel(c), nil
}

// ── post ──

type postInput struct {
	Channel     string `json:"channel"                jsonschema:"channel name or id to post to"`
	From        string `json:"from"                   jsonschema:"who is speaking (required)"`
	To          string `json:"to,omitempty"           jsonschema:"roster name this message is addressed to; set it on questions and tasks whenever more than two agents share the channel"`
	Body        string `json:"body"                   jsonschema:"the message (required)"`
	Kind        string `json:"kind,omitempty"         jsonschema:"intent of the message: task, result, question, answer, ack, or note; omit for a plain message"`
	ReplyTo     int64  `json:"reply_to,omitempty"     jsonschema:"seq of the message this answers — set it on every response, one reply_to per obligation"`
	ReplyNeeded bool   `json:"reply_needed,omitempty" jsonschema:"true when you are blocked until someone answers; the message stays in awaiting_reply until another message names it in reply_to"`
}

type postOutput struct {
	Message client.Message `json:"message"`
	Cursor  int64          `json:"cursor"  jsonschema:"the posted message's sequence number"`
	URL     string         `json:"url"`
}

func (h *handlers) handlePost(
	ctx context.Context,
	_ *mcp.CallToolRequest,
	in postInput,
) (*mcp.CallToolResult, postOutput, error) {
	m, err := h.client.Post(ctx, in.Channel, client.PostBody{
		From:        in.From,
		To:          in.To,
		Body:        in.Body,
		Kind:        in.Kind,
		ReplyTo:     in.ReplyTo,
		ReplyNeeded: in.ReplyNeeded,
	})
	if err != nil {
		return nil, postOutput{}, err
	}
	return nil, postOutput{Message: m, Cursor: m.Seq, URL: h.channelURL(in.Channel)}, nil
}

// ── read ──

type readInput struct {
	Channel string `json:"channel"         jsonschema:"channel name or id to read"`
	Since   int64  `json:"since,omitempty" jsonschema:"cursor from your last read; omit or 0 to read from the start"`
	Wait    int    `json:"wait,omitempty"  jsonschema:"seconds to block waiting for a new message, up to 120; omit or 0 to return whatever is there right now"`
	Limit   int    `json:"limit,omitempty" jsonschema:"maximum messages to return; omit for no limit"`
}

type readOutput struct {
	Channel                client.Channel     `json:"channel"`
	Connect                string             `json:"connect"                             jsonschema:"the connection string for this channel"`
	Messages               []client.Message   `json:"messages"`
	Cursor                 int64              `json:"cursor"                              jsonschema:"pass this as the since argument on your next read"`
	Members                []string           `json:"members,omitempty"                   jsonschema:"the roster: everyone currently on the channel"`
	AwaitingReply          []int64            `json:"awaiting_reply,omitempty"            jsonschema:"every seq posted with reply_needed that nothing has answered yet"`
	AwaitingReplyBy        map[string][]int64 `json:"awaiting_reply_by,omitempty"         jsonschema:"open obligations grouped by the roster name they are addressed to — settle the ones under your name before posting anything new"`
	AwaitingReplyOffRoster []string           `json:"awaiting_reply_off_roster,omitempty" jsonschema:"addressees in awaiting_reply_by who are not on the roster — a typo'd name, a departed member, or an agent yet to join. When non-empty, read the seqs under those names: a question misaddressed to you sits under a key you would never check, so awaiting_reply_by alone can mislead"`
	Closed                 bool               `json:"closed"                              jsonschema:"true when the channel is finished and will never produce another message"`
	URL                    string             `json:"url"`
}

func (h *handlers) handleRead(
	ctx context.Context,
	_ *mcp.CallToolRequest,
	in readInput,
) (*mcp.CallToolResult, readOutput, error) {
	wait := min(in.Wait, maxWaitSeconds)
	b, err := h.client.Read(
		ctx,
		in.Channel,
		client.ReadOptions{Since: in.Since, Limit: in.Limit, Wait: wait},
	)
	if err != nil {
		return nil, readOutput{}, err
	}
	return nil, readOutput{
		Channel:                b.Channel,
		Connect:                ref.String(h.port, b.Channel.Name),
		Messages:               b.Messages,
		Cursor:                 b.Cursor,
		Members:                b.Members,
		AwaitingReply:          b.AwaitingReply,
		AwaitingReplyBy:        b.AwaitingReplyBy,
		AwaitingReplyOffRoster: b.AwaitingReplyOffRoster,
		Closed:                 b.Channel.Closed(),
		URL:                    h.channelURL(b.Channel.Name),
	}, nil
}

// ── list ──

type listInput struct {
	IncludeClosed bool `json:"include_closed,omitempty" jsonschema:"include finished conversations as well as live ones"`
}

type listOutput struct {
	Channels []client.Summary `json:"channels"`
	URL      string           `json:"url"`
}

func (h *handlers) handleList(
	ctx context.Context,
	_ *mcp.CallToolRequest,
	in listInput,
) (*mcp.CallToolResult, listOutput, error) {
	cs, err := h.client.List(ctx, in.IncludeClosed)
	if err != nil {
		return nil, listOutput{}, err
	}
	return nil, listOutput{Channels: cs, URL: h.webURL}, nil
}

// ── close ──

type closeInput struct {
	Channel string `json:"channel"        jsonschema:"channel name or id to close"`
	Note    string `json:"note,omitempty" jsonschema:"optional parting note: how the conversation ended"`
}

func (h *handlers) handleClose(
	ctx context.Context,
	_ *mcp.CallToolRequest,
	in closeInput,
) (*mcp.CallToolResult, channelOut, error) {
	c, err := h.client.Close(ctx, in.Channel, in.Note)
	if err != nil {
		return nil, channelOut{}, err
	}
	return nil, h.channel(c), nil
}
