package bot

import (
    "strings"
    "testing"
    "time"
)

func TestFormatContestDiscordMessage(t *testing.T) {
    now := time.Now().Unix()
    info := Info{
        ID:               1,
        Name:             "Test Contest",
        Type:             "CF",
        DurationSeconds:  90 * 60,
        StartTimeSeconds: now + 24*3600, 
        EndTimeSeconds:   now + 24*3600 + 90*60,
    }

    msg := formatContestDiscordMessage(info, time.Until(time.Unix(info.StartTimeSeconds, 0)))
    if !strings.Contains(msg, "Test Contest") {
        t.Fatalf("message missing name: %q", msg)
    }
    if !strings.Contains(msg, "Type: CF") {
        t.Fatalf("message missing type: %q", msg)
    }
    if strings.Contains(msg, "End time") {
        t.Fatalf("message must not include 'End time', got: %q", msg)
    }
    if !strings.Contains(msg, "Link: https://codeforces.com/contests") {
        t.Fatalf("message missing link: %q", msg)
    }
}

func TestSplitDiscordMessageParts(t *testing.T) {
    lines := make([]string, 0)
    for i := 0; i < 50; i++ {
        lines = append(lines, strings.Repeat("A", 80))
    }

    parts := splitDiscordMessageParts(lines, 500)
    if len(parts) < 2 {
        t.Fatalf("expected multiple parts, got %d", len(parts))
    }
    joined := strings.Join(parts, "\n\n")
    expected := strings.Join(lines, "\n\n")
    if joined != expected {
        t.Fatalf("split/join mismatch")
    }
}
