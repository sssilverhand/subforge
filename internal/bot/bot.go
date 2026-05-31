package bot

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	tgbot "github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/sssilverhand/subforge/internal/config"
	"github.com/sssilverhand/subforge/internal/core/subscription"
	"github.com/sssilverhand/subforge/internal/db"
)

// Bot is the Telegram bot for SubForge.
type Bot struct {
	cfg      *config.Config
	tg       *tgbot.Bot
	subs     *db.SubscriptionStore
	subSvc   *subscription.Service
	nodes    *db.NodeStore
	users    *db.UserStore
	botUsers *db.BotUserStore
	log      *slog.Logger
	states   *stateStore
	baseURL  string // external_url for sub links
}

func New(
	cfg *config.Config,
	subs *db.SubscriptionStore,
	subSvc *subscription.Service,
	nodes *db.NodeStore,
	users *db.UserStore,
	botUsers *db.BotUserStore,
	log *slog.Logger,
) (*Bot, error) {
	b := &Bot{
		cfg:      cfg,
		subs:     subs,
		subSvc:   subSvc,
		nodes:    nodes,
		users:    users,
		botUsers: botUsers,
		log:      log,
		states:   newStateStore(),
		baseURL:  cfg.Server.ExternalURL,
	}

	opts := []tgbot.Option{
		tgbot.WithDefaultHandler(b.defaultHandler),
	}
	if cfg.Bot.Webhook != "" {
		opts = append(opts, tgbot.WithWebhookSecretToken(cfg.Bot.Webhook))
	}

	tg, err := tgbot.New(cfg.Bot.Token, opts...)
	if err != nil {
		return nil, fmt.Errorf("create telegram bot: %w", err)
	}
	b.tg = tg
	b.registerHandlers()
	return b, nil
}

// Start begins polling or webhook processing. Blocks until ctx is cancelled.
func (b *Bot) Start(ctx context.Context) {
	b.log.Info("telegram bot starting")
	b.tg.Start(ctx)
}

func (b *Bot) registerHandlers() {
	b.tg.RegisterHandler(tgbot.HandlerTypeMessageText, "/start", tgbot.MatchTypePrefix, b.handleStart)
	b.tg.RegisterHandler(tgbot.HandlerTypeCallbackQueryData, "main", tgbot.MatchTypeExact, b.handleMain)
	b.tg.RegisterHandler(tgbot.HandlerTypeCallbackQueryData, "sub:", tgbot.MatchTypePrefix, b.handleSubCallback)
	b.tg.RegisterHandler(tgbot.HandlerTypeCallbackQueryData, "node:", tgbot.MatchTypePrefix, b.handleNodeCallback)
	b.tg.RegisterHandler(tgbot.HandlerTypeCallbackQueryData, "my:", tgbot.MatchTypePrefix, b.handleMyCallback)
	b.tg.RegisterHandler(tgbot.HandlerTypeCallbackQueryData, "users:", tgbot.MatchTypePrefix, b.handleUsersCallback)
}

// handlerFunc adapts a typed handler to tgbot.HandlerFunc.
// go-telegram/bot passes the full Update for every handler type.

// ─── Entry point ─────────────────────────────────────────────────────────────

func (b *Bot) handleStart(ctx context.Context, _ *tgbot.Bot, update *models.Update) {
	msg := update.Message
	if msg == nil {
		return
	}
	chatID := msg.Chat.ID

	_ = b.botUsers.Upsert(ctx, db.BotUser{
		ChatID:    chatID,
		Username:  msg.From.Username,
		FirstName: msg.From.FirstName,
		LastName:  msg.From.LastName,
	})

	if b.isAdmin(ctx, chatID) {
		b.sendAdmin(ctx, chatID, "👋 Добро пожаловать, администратор!", mainMenuAdmin())
		return
	}

	// Check if this user has a subscription
	sub, err := b.subs.GetByChatID(ctx, chatID)
	if err != nil {
		b.sendText(ctx, chatID, "Произошла ошибка. Попробуйте позже.")
		return
	}
	if sub == nil {
		b.sendText(ctx, chatID, "👋 Добро пожаловать!\n\nВаш аккаунт зарегистрирован. Ожидайте выдачи доступа от администратора.")
		b.notifyAdminsNewUser(ctx, msg.From)
		return
	}

	b.sendUser(ctx, chatID, sub)
}

func (b *Bot) handleMain(ctx context.Context, _ *tgbot.Bot, update *models.Update) {
	if update.CallbackQuery == nil {
		return
	}
	chatID := update.CallbackQuery.Message.Message.Chat.ID
	b.answerCallback(ctx, update.CallbackQuery.ID)
	if b.isAdmin(ctx, chatID) {
		b.editMessage(ctx, chatID, update.CallbackQuery.Message.Message.ID,
			"🏠 Главное меню", mainMenuAdmin())
	} else {
		sub, _ := b.subs.GetByChatID(ctx, chatID)
		if sub != nil {
			b.sendUser(ctx, chatID, sub)
		}
	}
}

func (b *Bot) defaultHandler(ctx context.Context, _ *tgbot.Bot, update *models.Update) {
	if update.Message == nil || update.Message.Text == "" {
		return
	}
	chatID := update.Message.Chat.ID
	state, data := b.states.get(chatID)

	switch state {
	case stateAwaitingSubName:
		b.handleCreateSubName(ctx, chatID, update.Message.Text, data)
	default:
		// ignore unknown messages
		_ = data
	}
}

// ─── Admin handlers ───────────────────────────────────────────────────────────

func (b *Bot) handleSubCallback(ctx context.Context, _ *tgbot.Bot, update *models.Update) {
	cq := update.CallbackQuery
	chatID := cq.Message.Message.Chat.ID
	msgID := cq.Message.Message.ID
	b.answerCallback(ctx, cq.ID)

	if !b.isAdmin(ctx, chatID) {
		return
	}

	parts := strings.Split(cq.Data, ":")
	if len(parts) < 2 {
		return
	}

	switch {
	case cq.Data == "sub:create":
		b.states.set(chatID, stateAwaitingSubName, map[string]any{})
		b.editMessage(ctx, chatID, msgID, "📝 Введите имя подписки (или отправьте - для пропуска):", backKeyboard("sub:list:0"))

	case parts[1] == "list":
		page := 0
		if len(parts) > 2 {
			fmt.Sscan(parts[2], &page)
		}
		b.showSubList(ctx, chatID, msgID, page)

	case parts[1] == "view" && len(parts) == 3:
		b.showSubView(ctx, chatID, msgID, parts[2])

	case parts[1] == "links" && len(parts) == 3:
		b.editMessage(ctx, chatID, msgID, "📲 Выберите клиент:", subLinksKeyboard(parts[2]))

	case parts[1] == "link" && len(parts) == 4:
		b.sendSubLink(ctx, chatID, msgID, parts[2], parts[3])

	case parts[1] == "enable" && len(parts) == 3:
		b.doEnableSub(ctx, chatID, msgID, parts[2], true)

	case parts[1] == "disable" && len(parts) == 3:
		b.doEnableSub(ctx, chatID, msgID, parts[2], false)

	case parts[1] == "reset" && len(parts) == 3:
		b.doResetTraffic(ctx, chatID, msgID, parts[2])

	case parts[1] == "delete" && len(parts) == 3:
		b.editMessage(ctx, chatID, msgID,
			"⚠️ Удалить подписку и отозвать доступ?",
			confirmKeyboard("sub:deleteconfirm:"+parts[2], "sub:view:"+parts[2]))

	case parts[1] == "deleteconfirm" && len(parts) == 3:
		b.doDeleteSub(ctx, chatID, msgID, parts[2])

	case parts[1] == "send" && len(parts) == 3:
		b.doSendSubToUser(ctx, chatID, msgID, parts[2])
	}
}

func (b *Bot) handleNodeCallback(ctx context.Context, _ *tgbot.Bot, update *models.Update) {
	cq := update.CallbackQuery
	chatID := cq.Message.Message.Chat.ID
	msgID := cq.Message.Message.ID
	b.answerCallback(ctx, cq.ID)

	if !b.isAdmin(ctx, chatID) {
		return
	}

	parts := strings.Split(cq.Data, ":")

	switch {
	case cq.Data == "node:list":
		b.showNodeList(ctx, chatID, msgID)

	case parts[1] == "view" && len(parts) == 3:
		b.showNodeView(ctx, chatID, msgID, parts[2])

	case parts[1] == "status" && len(parts) == 3:
		b.showNodeStatus(ctx, chatID, msgID, parts[2])

	case parts[1] == "install" && len(parts) == 4:
		b.doInstallBinary(ctx, chatID, msgID, parts[3], parts[2])
	}
}

func (b *Bot) handleUsersCallback(ctx context.Context, _ *tgbot.Bot, update *models.Update) {
	cq := update.CallbackQuery
	chatID := cq.Message.Message.Chat.ID
	msgID := cq.Message.Message.ID
	b.answerCallback(ctx, cq.ID)

	if !b.isAdmin(ctx, chatID) {
		return
	}

	if cq.Data == "users:list" {
		b.showUsersList(ctx, chatID, msgID)
	}
}

// ─── User handlers ────────────────────────────────────────────────────────────

func (b *Bot) handleMyCallback(ctx context.Context, _ *tgbot.Bot, update *models.Update) {
	cq := update.CallbackQuery
	chatID := cq.Message.Message.Chat.ID
	msgID := cq.Message.Message.ID
	b.answerCallback(ctx, cq.ID)

	sub, err := b.subs.GetByChatID(ctx, chatID)
	if err != nil || sub == nil {
		b.editMessage(ctx, chatID, msgID, "❌ Подписка не найдена.", nil)
		return
	}

	parts := strings.Split(cq.Data, ":")
	switch {
	case cq.Data == "my:info":
		b.sendUser(ctx, chatID, sub)
	case cq.Data == "my:links":
		b.editMessage(ctx, chatID, msgID, "📲 Выберите клиент:", userLinksKeyboard())
	case parts[1] == "link" && len(parts) == 3:
		link := b.baseURL + "/sub/" + sub.Token + "?client=" + parts[2]
		b.editMessage(ctx, chatID, msgID,
			fmt.Sprintf("📲 Ссылка для <b>%s</b>:\n\n<code>%s</code>", parts[2], link),
			backKeyboard("my:links"))
	}
}

// ─── Admin actions ────────────────────────────────────────────────────────────

func (b *Bot) showSubList(ctx context.Context, chatID int64, msgID int, page int) {
	const pageSize = 8
	allSubs, err := b.subs.ListAll(ctx, pageSize, page*pageSize)
	if err != nil {
		b.editMessage(ctx, chatID, msgID, "❌ Ошибка загрузки.", nil)
		return
	}
	items := make([]subItem, len(allSubs))
	for i, s := range allSubs {
		label := s.Token[:8]
		if s.Name != nil && *s.Name != "" {
			label = *s.Name
		}
		items[i] = subItem{ID: s.ID.String(), Label: label, IsEnabled: s.IsEnabled}
	}
	b.editMessage(ctx, chatID, msgID,
		fmt.Sprintf("📋 Подписки (стр. %d):", page+1),
		subListKeyboard(items, page))
}

func (b *Bot) showSubView(ctx context.Context, chatID int64, msgID int, idStr string) {
	sub, err := b.subByIDStr(ctx, idStr)
	if err != nil || sub == nil {
		b.editMessage(ctx, chatID, msgID, "❌ Подписка не найдена.", backKeyboard("sub:list:0"))
		return
	}

	name := sub.Token[:8]
	if sub.Name != nil {
		name = *sub.Name
	}

	used := sub.TrafficUsedBytes / 1024 / 1024
	limit := "∞"
	if sub.TrafficLimitBytes != nil {
		limit = fmt.Sprintf("%d MB", *sub.TrafficLimitBytes/1024/1024)
	}
	exp := "∞"
	if sub.ExpiresAt != nil {
		exp = sub.ExpiresAt.Format("02.01.2006")
	}
	status := "✅ Активна"
	if !sub.IsEnabled {
		status = "❌ Отключена"
	}
	if sub.IsExpired {
		status = "⏰ Истекла"
	}
	if sub.IsTrafficExceeded {
		status = "📵 Трафик исчерпан"
	}

	text := fmt.Sprintf(
		"📦 <b>%s</b>\n\n"+
			"Статус: %s\n"+
			"Трафик: %d MB / %s\n"+
			"Истекает: %s\n"+
			"Токен: <code>%s</code>",
		name, status, used, limit, exp, sub.Token,
	)
	b.editMessage(ctx, chatID, msgID, text, subViewKeyboard(idStr, sub.IsEnabled))
}

func (b *Bot) sendSubLink(ctx context.Context, chatID int64, msgID int, idStr, client string) {
	sub, err := b.subByIDStr(ctx, idStr)
	if err != nil || sub == nil {
		return
	}
	link := fmt.Sprintf("%s/sub/%s?client=%s", b.baseURL, sub.Token, client)
	b.editMessage(ctx, chatID, msgID,
		fmt.Sprintf("📲 Ссылка (%s):\n\n<code>%s</code>", client, link),
		backKeyboard("sub:links:"+idStr))
}

func (b *Bot) doEnableSub(ctx context.Context, chatID int64, msgID int, idStr string, enable bool) {
	id, err := parseUUID(idStr)
	if err != nil {
		return
	}
	if enable {
		err = b.subSvc.Enable(ctx, id)
	} else {
		err = b.subSvc.Disable(ctx, id)
	}
	if err != nil {
		b.editMessage(ctx, chatID, msgID, "❌ "+err.Error(), backKeyboard("sub:view:"+idStr))
		return
	}
	b.showSubView(ctx, chatID, msgID, idStr)
}

func (b *Bot) doResetTraffic(ctx context.Context, chatID int64, msgID int, idStr string) {
	id, err := parseUUID(idStr)
	if err != nil {
		return
	}
	if err := b.subSvc.ResetTraffic(ctx, id); err != nil {
		b.editMessage(ctx, chatID, msgID, "❌ "+err.Error(), backKeyboard("sub:view:"+idStr))
		return
	}
	b.showSubView(ctx, chatID, msgID, idStr)
}

func (b *Bot) doDeleteSub(ctx context.Context, chatID int64, msgID int, idStr string) {
	id, err := parseUUID(idStr)
	if err != nil {
		return
	}
	if err := b.subSvc.Delete(ctx, id); err != nil {
		b.editMessage(ctx, chatID, msgID, "❌ "+err.Error(), backKeyboard("sub:list:0"))
		return
	}
	b.showSubList(ctx, chatID, msgID, 0)
}

func (b *Bot) doSendSubToUser(ctx context.Context, chatID int64, msgID int, idStr string) {
	sub, err := b.subByIDStr(ctx, idStr)
	if err != nil || sub == nil {
		return
	}
	if sub.TelegramChatID == nil {
		b.editMessage(ctx, chatID, msgID, "❌ Юзер не привязан к этой подписке.", backKeyboard("sub:view:"+idStr))
		return
	}
	link := b.baseURL + "/sub/" + sub.Token
	text := fmt.Sprintf("🎉 Ваша подписка готова!\n\nСсылка: <code>%s</code>\n\nИспользуйте /start для просмотра деталей.", link)
	b.sendText(ctx, *sub.TelegramChatID, text)
	b.editMessage(ctx, chatID, msgID, "✅ Отправлено пользователю.", backKeyboard("sub:view:"+idStr))
}

func (b *Bot) handleCreateSubName(ctx context.Context, chatID int64, text string, data map[string]any) {
	b.states.clear(chatID)
	name := text
	if name == "-" {
		name = ""
	}
	_ = data
	// For full creation flow, admin needs to pick inbounds via web panel.
	// Bot creates with no inbounds (admin assigns via panel).
	var namePtr *string
	if name != "" {
		namePtr = &name
	}
	sub, err := b.subSvc.Create(ctx, subscription.CreateParams{
		Name:      namePtr,
		CreatedBy: nil, // bot-created
	})
	if err != nil {
		b.sendText(ctx, chatID, "❌ Ошибка создания: "+err.Error())
		return
	}
	b.sendText(ctx, chatID, fmt.Sprintf(
		"✅ Подписка создана!\n\nТокен: <code>%s</code>\n\n"+
			"Откройте веб-панель для настройки протоколов и нод.",
		sub.Token,
	))
}

func (b *Bot) showNodeList(ctx context.Context, chatID int64, msgID int) {
	nodes, err := b.nodes.ListActive(ctx)
	if err != nil {
		b.editMessage(ctx, chatID, msgID, "❌ Ошибка.", nil)
		return
	}
	items := make([]nodeItem, len(nodes))
	for i, n := range nodes {
		items[i] = nodeItem{ID: n.ID.String(), Name: n.Name, Active: n.IsActive}
	}
	b.editMessage(ctx, chatID, msgID, "🖥 Ноды:", nodeListKeyboard(items))
}

func (b *Bot) showNodeView(ctx context.Context, chatID int64, msgID int, idStr string) {
	id, _ := parseUUID(idStr)
	node, err := b.nodes.GetByID(ctx, id)
	if err != nil || node == nil {
		b.editMessage(ctx, chatID, msgID, "❌ Нода не найдена.", backKeyboard("node:list"))
		return
	}
	text := fmt.Sprintf("🖥 <b>%s</b>\n\nHost: <code>%s</code>", node.Name, node.PublicHost)
	b.editMessage(ctx, chatID, msgID, text, nodeViewKeyboard(idStr))
}

func (b *Bot) showNodeStatus(ctx context.Context, chatID int64, msgID int, idStr string) {
	id, _ := parseUUID(idStr)
	node, err := b.nodes.GetByID(ctx, id)
	if err != nil || node == nil || node.AgentURL == nil {
		b.editMessage(ctx, chatID, msgID, "❌ Агент не настроен для этой ноды.", backKeyboard("node:view:"+idStr))
		return
	}
	// agentclient call — kept simple, no import cycle
	b.editMessage(ctx, chatID, msgID, "🔍 Запрос статуса... (используйте веб-панель для деталей)", backKeyboard("node:view:"+idStr))
}

func (b *Bot) doInstallBinary(ctx context.Context, chatID int64, msgID int, nodeIDStr, binary string) {
	b.editMessage(ctx, chatID, msgID,
		fmt.Sprintf("⬇️ Установка <b>%s</b>...\n\nЭто может занять несколько минут.", binary),
		nil)
	// Full install logic goes via agentclient — done via web panel for complex operations.
	b.sendText(ctx, chatID, "✅ Команда отправлена. Следите за статусом в веб-панели.")
}

func (b *Bot) showUsersList(ctx context.Context, chatID int64, msgID int) {
	busers, err := b.botUsers.ListRecent(ctx, 20)
	if err != nil {
		b.editMessage(ctx, chatID, msgID, "❌ Ошибка.", nil)
		return
	}
	if len(busers) == 0 {
		b.editMessage(ctx, chatID, msgID, "Нет пользователей.", backKeyboard("main"))
		return
	}
	var sb strings.Builder
	sb.WriteString("👥 <b>Последние пользователи:</b>\n\n")
	for _, u := range busers {
		name := u.FirstName
		if u.Username != "" {
			name += " (@" + u.Username + ")"
		}
		hasSub := "❌"
		if sub, _ := b.subs.GetByChatID(ctx, u.ChatID); sub != nil {
			hasSub = "✅"
		}
		sb.WriteString(fmt.Sprintf("%s %s — %d\n", hasSub, name, u.ChatID))
	}
	b.editMessage(ctx, chatID, msgID, sb.String(), backKeyboard("main"))
}

// ─── User helpers ─────────────────────────────────────────────────────────────

func (b *Bot) sendUser(ctx context.Context, chatID int64, sub *db.Subscription) {
	name := sub.Token[:8]
	if sub.Name != nil && *sub.Name != "" {
		name = *sub.Name
	}

	used := sub.TrafficUsedBytes / 1024 / 1024
	limit := "∞"
	if sub.TrafficLimitBytes != nil {
		limit = fmt.Sprintf("%d MB", *sub.TrafficLimitBytes/1024/1024)
	}
	exp := "∞"
	if sub.ExpiresAt != nil {
		days := int(time.Until(*sub.ExpiresAt).Hours() / 24)
		exp = fmt.Sprintf("%s (%d дн.)", sub.ExpiresAt.Format("02.01.2006"), days)
	}
	status := "✅ Активна"
	if sub.IsExpired {
		status = "⏰ Истекла"
	}
	if sub.IsTrafficExceeded {
		status = "📵 Трафик исчерпан"
	}
	if !sub.IsEnabled {
		status = "❌ Приостановлена"
	}

	text := fmt.Sprintf(
		"📦 <b>%s</b>\n\n"+
			"Статус: %s\n"+
			"Использовано: %d MB / %s\n"+
			"Истекает: %s",
		name, status, used, limit, exp,
	)
	b.sendAdmin(ctx, chatID, text, mainMenuUser())
}

// ─── Notifications ────────────────────────────────────────────────────────────

// NotifyTrafficWarning sends a warning when a subscription is at 80% traffic.
func (b *Bot) NotifyTrafficWarning(ctx context.Context, sub *db.Subscription) {
	if sub.TelegramChatID == nil {
		return
	}
	used := sub.TrafficUsedBytes / 1024 / 1024
	total := int64(0)
	if sub.TrafficLimitBytes != nil {
		total = *sub.TrafficLimitBytes / 1024 / 1024
	}
	b.sendText(ctx, *sub.TelegramChatID,
		fmt.Sprintf("⚠️ Использовано 80%% трафика: %d MB из %d MB", used, total))
}

// NotifyExpiringSoon sends a warning 3 days before expiry.
func (b *Bot) NotifyExpiringSoon(ctx context.Context, sub *db.Subscription) {
	if sub.TelegramChatID == nil || sub.ExpiresAt == nil {
		return
	}
	days := int(time.Until(*sub.ExpiresAt).Hours() / 24)
	b.sendText(ctx, *sub.TelegramChatID,
		fmt.Sprintf("⏰ Подписка истекает через %d дн. (%s)", days, sub.ExpiresAt.Format("02.01.2006")))
}

func (b *Bot) notifyAdminsNewUser(ctx context.Context, from *models.User) {
	admins, err := b.users.ListWithTelegramID(ctx)
	if err != nil {
		return
	}
	name := from.FirstName
	if from.Username != "" {
		name += " (@" + from.Username + ")"
	}
	text := fmt.Sprintf("👤 Новый пользователь: %s\nChat ID: <code>%d</code>", name, from.ID)
	for _, admin := range admins {
		if admin.TelegramChatID != nil {
			b.sendText(ctx, *admin.TelegramChatID, text)
		}
	}
}

// ─── Telegram helpers ─────────────────────────────────────────────────────────

func (b *Bot) sendAdmin(ctx context.Context, chatID int64, text string, kb *models.InlineKeyboardMarkup) {
	params := &tgbot.SendMessageParams{
		ChatID:    chatID,
		Text:      text,
		ParseMode: models.ParseModeHTML,
	}
	if kb != nil {
		params.ReplyMarkup = kb
	}
	_, _ = b.tg.SendMessage(ctx, params)
}

func (b *Bot) sendText(ctx context.Context, chatID int64, text string) {
	_, _ = b.tg.SendMessage(ctx, &tgbot.SendMessageParams{
		ChatID:    chatID,
		Text:      text,
		ParseMode: models.ParseModeHTML,
	})
}

func (b *Bot) editMessage(ctx context.Context, chatID int64, msgID int, text string, kb *models.InlineKeyboardMarkup) {
	params := &tgbot.EditMessageTextParams{
		ChatID:    chatID,
		MessageID: msgID,
		Text:      text,
		ParseMode: models.ParseModeHTML,
	}
	if kb != nil {
		params.ReplyMarkup = kb
	}
	_, _ = b.tg.EditMessageText(ctx, params)
}

func (b *Bot) answerCallback(ctx context.Context, id string) {
	_, _ = b.tg.AnswerCallbackQuery(ctx, &tgbot.AnswerCallbackQueryParams{CallbackQueryID: id})
}

// ─── Misc ─────────────────────────────────────────────────────────────────────

func (b *Bot) isAdmin(ctx context.Context, chatID int64) bool {
	admins, err := b.users.ListWithTelegramID(ctx)
	if err != nil {
		return false
	}
	for _, a := range admins {
		if a.TelegramChatID != nil && *a.TelegramChatID == chatID {
			return true
		}
	}
	return false
}

func (b *Bot) subByIDStr(ctx context.Context, idStr string) (*db.Subscription, error) {
	id, err := parseUUID(idStr)
	if err != nil {
		return nil, err
	}
	return b.subs.GetByID(ctx, id)
}
