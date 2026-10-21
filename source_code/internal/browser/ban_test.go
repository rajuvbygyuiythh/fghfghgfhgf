package browser

import "testing"

func TestParseBanText(t *testing.T) {
	text := "Chitchat.gg\nAccount suspended\nYour access to Chitchat has been restricted.\nBan ID\n6a72c1792005f83bc98491c8\nReason\nBreaking our community guidelines and terms of service.\nRemaining\nPermanent\nUser ID: 6a72c70cca8c0a276af0b99\nContact support\n"
	report := ParseBanText(text, "https://app.chitchat.gg/banned")
	if !report.Banned || report.BanID != "6a72c1792005f83bc98491c8" {
		t.Fatalf("report=%+v", report)
	}
	if report.UserID != "6a72c70cca8c0a276af0b99" {
		t.Fatalf("user id=%q", report.UserID)
	}
	if report.Reason != "Breaking our community guidelines and terms of service." || report.Remaining != "Permanent" {
		t.Fatalf("report=%+v", report)
	}
	if summary := report.Summary(); summary == "" || summary == "account suspended" {
		t.Fatalf("summary=%q", summary)
	}
}

func TestIsBannedURL(t *testing.T) {
	for _, raw := range []string{
		"https://app.chitchat.gg/banned",
		"https://app.chitchat.gg/banned/",
		"https://app.chitchat.gg/banned?x=1",
		"app.chitchat.gg/banned",
		"/banned",
	} {
		if !isBannedURL(raw) {
			t.Fatalf("%q should be banned", raw)
		}
	}
	for _, raw := range []string{"", "https://app.chitchat.gg/text", "/banned-users", "https://example.com/"} {
		if isBannedURL(raw) {
			t.Fatalf("%q should not be banned", raw)
		}
	}
}
