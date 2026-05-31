package bot

import "github.com/go-telegram/bot/models"

func mainMenuAdmin() *models.InlineKeyboardMarkup {
	return kb([][]btn{
		{b("📋 Подписки", "sub:list:0"), b("🖥 Ноды", "node:list")},
		{b("👥 Пользователи", "users:list"), b("⚙️ Настройки", "settings")},
	})
}

func mainMenuUser() *models.InlineKeyboardMarkup {
	return kb([][]btn{
		{b("📦 Моя подписка", "my:info")},
		{b("📲 Получить ссылки", "my:links")},
	})
}

func subListKeyboard(subs []subItem, page int) *models.InlineKeyboardMarkup {
	rows := make([][]btn, 0, len(subs)+1)
	for _, s := range subs {
		icon := "✅"
		if !s.IsEnabled {
			icon = "❌"
		}
		rows = append(rows, []btn{b(icon+" "+s.Label, "sub:view:"+s.ID)})
	}
	nav := []btn{}
	if page > 0 {
		nav = append(nav, b("◀️", "sub:list:"+itoa(page-1)))
	}
	nav = append(nav, b("➕ Создать", "sub:create"))
	rows = append(rows, nav)
	rows = append(rows, []btn{b("🏠 Главная", "main")})
	return kb(rows)
}

func subViewKeyboard(id string, enabled bool) *models.InlineKeyboardMarkup {
	toggle := b("⏸ Отключить", "sub:disable:"+id)
	if !enabled {
		toggle = b("▶️ Включить", "sub:enable:"+id)
	}
	return kb([][]btn{
		{b("📲 Ссылки", "sub:links:"+id)},
		{toggle, b("🔄 Сбросить трафик", "sub:reset:"+id)},
		{b("📤 Отправить юзеру", "sub:send:"+id), b("🗑 Удалить", "sub:delete:"+id)},
		{b("◀️ Назад", "sub:list:0")},
	})
}

func subLinksKeyboard(id string) *models.InlineKeyboardMarkup {
	return kb([][]btn{
		{b("Hiddify / Sing-box", "sub:link:"+id+":singbox")},
		{b("Clash Meta / Verge", "sub:link:"+id+":clash")},
		{b("v2rayN / NekoRay (raw)", "sub:link:"+id+":raw")},
		{b("◀️ Назад", "sub:view:"+id)},
	})
}

func userLinksKeyboard() *models.InlineKeyboardMarkup {
	return kb([][]btn{
		{b("Hiddify / Sing-box", "my:link:singbox")},
		{b("Clash Meta / Verge", "my:link:clash")},
		{b("v2rayN / NekoRay (raw)", "my:link:raw")},
		{b("◀️ Назад", "my:info")},
	})
}

func nodeListKeyboard(nodes []nodeItem) *models.InlineKeyboardMarkup {
	rows := make([][]btn, 0, len(nodes)+1)
	for _, n := range nodes {
		icon := "🟢"
		if !n.Active {
			icon = "🔴"
		}
		rows = append(rows, []btn{b(icon+" "+n.Name, "node:view:"+n.ID)})
	}
	rows = append(rows, []btn{b("🏠 Главная", "main")})
	return kb(rows)
}

func nodeViewKeyboard(id string) *models.InlineKeyboardMarkup {
	return kb([][]btn{
		{b("📥 Установить xray", "node:install:xray:"+id)},
		{b("📥 Установить hysteria2", "node:install:hy2:"+id)},
		{b("🔄 Статус сервисов", "node:status:"+id)},
		{b("◀️ Назад", "node:list")},
	})
}

func confirmKeyboard(yes, no string) *models.InlineKeyboardMarkup {
	return kb([][]btn{{b("✅ Да", yes), b("❌ Отмена", no)}})
}

func backKeyboard(cb string) *models.InlineKeyboardMarkup {
	return kb([][]btn{{b("◀️ Назад", cb)}})
}

// ─── internal builders ───────────────────────────────────────────────────────

type btn struct {
	text string
	data string
}

func b(text, data string) btn { return btn{text, data} }

func kb(rows [][]btn) *models.InlineKeyboardMarkup {
	keyboard := make([][]models.InlineKeyboardButton, len(rows))
	for i, row := range rows {
		keyboard[i] = make([]models.InlineKeyboardButton, len(row))
		for j, b := range row {
			keyboard[i][j] = models.InlineKeyboardButton{Text: b.text, CallbackData: b.data}
		}
	}
	return &models.InlineKeyboardMarkup{InlineKeyboard: keyboard}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	s := ""
	for n > 0 {
		s = string(rune('0'+n%10)) + s
		n /= 10
	}
	return s
}

// lightweight view models for keyboards

type subItem struct {
	ID        string
	Label     string
	IsEnabled bool
}

type nodeItem struct {
	ID     string
	Name   string
	Active bool
}
