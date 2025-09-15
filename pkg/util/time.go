package util

import (
	"fmt"
	"time"
)

func TimeAgo(t time.Time) string {
	now := time.Now()
	duration := now.Sub(t)

	seconds := int(duration.Seconds())
	minutes := int(duration.Minutes())
	hours := int(duration.Hours())
	days := hours / 24
	weeks := days / 7
	months := days / 30
	years := days / 365

	switch {
	case seconds < 0:
		return "в будущем"
	case seconds < 10:
		return "только что"
	case seconds < 60:
		return "менее минуты назад"
	case minutes == 1:
		return "минуту назад"
	case minutes < 5:
		return fmt.Sprintf("%d минуты назад", minutes)
	case minutes < 60:
		return fmt.Sprintf("%d минут назад", minutes)
	case hours == 1:
		return "час назад"
	case hours < 5:
		return fmt.Sprintf("%d часа назад", hours)
	case hours < 24:
		return fmt.Sprintf("%d часов назад", hours)
	case days == 1:
		return "вчера"
	case days < 5:
		return fmt.Sprintf("%d дня назад", days)
	case days < 7:
		return fmt.Sprintf("%d дней назад", days)
	case weeks == 1:
		return "неделю назад"
	case weeks < 4:
		return fmt.Sprintf("%d недели назад", weeks)
	case months == 1:
		return "месяц назад"
	case months < 12:
		return fmt.Sprintf("%d месяцев назад", months)
	case years == 1:
		return "год назад"
	case years < 5:
		return fmt.Sprintf("%d года назад", years)
	default:
		return "давно"
	}
}
