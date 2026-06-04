package bot

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/bwmarrin/discordgo"
)

type Info struct {
	ID                  int    `json:"id"`
	Name                string `json:"name"`
	Type                string `json:"type"`
	Phase               string `json:"phase"`
	DurationSeconds     int    `json:"durationSeconds"`
	StartTimeSeconds    int64  `json:"startTimeSeconds"`
	EndTimeSeconds      int64  `json:"endTimeSeconds"`
	RelativeTimeSeconds int    `json:"relativeTimeSeconds"`
	Difficulty          int    `json:"difficulty"`
}

type contestListResponse struct {
	Status  string `json:"status"`
	Comment string `json:"comment"`
	Result  []Info `json:"result"`
}

var BotToken string

const (
	sendAvailableContestCommandName    = "sendavailablecontest"
	showAllAvailableContestCommandName = "showallavailablecontest"
	showAllAvailbleContestCommandName  = "showallavailblecontest"
)

var (
	sentContestsFile        = "sent_contests.json"
	contestMessageStoreFile = "sent_contest_messages.json"
	cachedCairoLocation     = mustLoadCairoLocation()
	sentContestsFileLock    sync.Mutex
	contestMessageStoreLock sync.Mutex
)

func environmentName() string {
	env := strings.TrimSpace(os.Getenv("ENVIRONMENT"))
	if env == "" {
		return "default"
	}
	return env
}

func init() {
	env := environmentName()
	sentContestsFile = fmt.Sprintf("sent_contests_%s.json", env)
	contestMessageStoreFile = fmt.Sprintf("sent_contest_messages_%s.json", env)
	log.Printf("using storage files %s and %s (ENVIRONMENT=%s)", sentContestsFile, contestMessageStoreFile, env)
	if !announcementsEnabled() {
		log.Println("announcements are disabled by environment (CI or DISABLE_ANNOUNCEMENTS)")
	}
	initDB()
}

type storedContestMessage struct {
	ChannelID string `json:"channelId"`
	Content   string `json:"content"`
}

func checkNilErr(err error) {
	if err != nil {
		log.Fatal(err)
	}
}

func mustLoadCairoLocation() *time.Location {
	location, err := time.LoadLocation("Africa/Cairo")
	if err != nil {
		return time.UTC
	}

	return location
}

func formatContestTime(unixSeconds int64) string {
	return time.Unix(unixSeconds, 0).In(cachedCairoLocation).Format("2006-01-02 15:04 MST")
}

func formatContestMessage(info Info) string {
	return formatContestDiscordMessage(info, time.Until(time.Unix(info.StartTimeSeconds, 0).In(cachedCairoLocation)))
}

func formatContestSummary(info Info) string {
	return formatContestDiscordMessage(info, 0)
}

func formatDailyAnnouncement(info Info) string {
	startTime := time.Unix(info.StartTimeSeconds, 0).In(cachedCairoLocation)
	remaining := time.Until(startTime)
	return formatContestDiscordMessage(info, remaining)
}

func formatContestDiscordMessage(info Info, remaining time.Duration) string {
	daysAvailable := 0
	if remaining > 0 {
		daysAvailable = int((remaining + 24*time.Hour - 1) / (24 * time.Hour))
		if daysAvailable < 1 {
			daysAvailable = 1
		}
	}

	message := fmt.Sprintf(
		"**%s**\nType: %s\nDuration: %d min\nStart time (Cairo): %s\nLink: https://codeforces.com/contests",
		info.Name,
		info.Type,
		info.DurationSeconds/60,
		formatContestTime(info.StartTimeSeconds),
	)

	if daysAvailable > 0 {
		message += fmt.Sprintf("\nStarts in: %d day(s)", daysAvailable)
	}

	return message
}

func storeContestMessage(messageID, channelID, content string) {
	if messageID == "" || channelID == "" || content == "" {
		return
	}

	contestMessageStoreLock.Lock()
	defer contestMessageStoreLock.Unlock()

	storedMessages := loadContestMessagesLocked()
	storedMessages[messageID] = storedContestMessage{
		ChannelID: channelID,
		Content:   content,
	}
	saveContestMessagesLocked(storedMessages)
}

func loadContestMessagesLocked() map[string]storedContestMessage {
	data, err := os.ReadFile(contestMessageStoreFile)
	if err != nil {
		return map[string]storedContestMessage{}
	}

	storedMessages := map[string]storedContestMessage{}
	if err := json.Unmarshal(data, &storedMessages); err != nil {
		return map[string]storedContestMessage{}
	}

	return storedMessages
}

func saveContestMessagesLocked(storedMessages map[string]storedContestMessage) {
	data, err := json.MarshalIndent(storedMessages, "", "  ")
	if err != nil {
		log.Println("failed to marshal contest messages:", err)
		return
	}

	if err := writeAtomic(contestMessageStoreFile, data, 0o644); err != nil {
		log.Println("failed to save contest messages:", err)
	}
}

func announcementChannelID() string {
	channelID := strings.TrimSpace(os.Getenv("DISCORD_ANNOUNCEMENT_CHANNEL_ID"))
	if channelID == "" {
		return ""
	}

	for _, character := range channelID {
		if character < '0' || character > '9' {
			return ""
		}
	}

	return channelID
}

func discordGuildID() string {
	guildID := strings.TrimSpace(os.Getenv("DISCORD_GUILD_ID"))
	if guildID == "" {
		return ""
	}

	for _, character := range guildID {
		if character < '0' || character > '9' {
			return ""
		}
	}

	return guildID
}

func upcomingContests() ([]Info, error) {
	contests, err := getContestList()
	if err != nil {
		return nil, err
	}

	now := time.Now().Unix()
	upcoming := make([]Info, 0, len(contests))
	for _, contest := range contests {
		if contest.Phase != "BEFORE" || contest.StartTimeSeconds <= now {
			continue
		}

		upcoming = append(upcoming, contest)
	}

	sort.Slice(upcoming, func(i, j int) bool {
		return upcoming[i].StartTimeSeconds < upcoming[j].StartTimeSeconds
	})

	return upcoming, nil
}

func splitDiscordMessageParts(lines []string, maxLength int) []string {
	if len(lines) == 0 {
		return nil
	}

	chunks := make([]string, 0)
	current := strings.Builder{}

	for _, line := range lines {
		candidate := line
		if current.Len() > 0 {
			candidate = current.String() + "\n\n" + line
		}

		if current.Len() > 0 && len(candidate) > maxLength {
			chunks = append(chunks, current.String())
			current.Reset()
		}

		if current.Len() > 0 {
			current.WriteString("\n\n")
		}
		current.WriteString(line)
	}

	if current.Len() > 0 {
		chunks = append(chunks, current.String())
	}

	return chunks
}

func registerApplicationCommands(discord *discordgo.Session) error {
	if discord.State == nil || discord.State.User == nil {
		return errors.New("discord state is not ready")
	}

	commands := []*discordgo.ApplicationCommand{
		{
			Name:        sendAvailableContestCommandName,
			Description: "Send all upcoming Codeforces contests",
		},
		{
			Name:        showAllAvailableContestCommandName,
			Description: "Show all upcoming Codeforces contests",
		},
		{
			Name:        showAllAvailbleContestCommandName,
			Description: "Show all upcoming Codeforces contests",
		},
	}

	guildID := discordGuildID()
	_, err := discord.ApplicationCommandBulkOverwrite(discord.State.User.ID, guildID, commands)
	return err
}

func handleReady(discord *discordgo.Session, ready *discordgo.Ready) {
	if err := registerApplicationCommands(discord); err != nil {
		log.Println("failed to register slash commands:", err)
	}

	startContestWatcher(discord)
}

func sendUpcomingContests(discord *discordgo.Session, interaction *discordgo.InteractionCreate) {
	if err := discord.InteractionRespond(interaction.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
	}); err != nil {
		log.Println("failed to defer interaction response:", err)
		return
	}

	contests, err := upcomingContests()
	if err != nil {
		_, _ = discord.FollowupMessageCreate(interaction.Interaction, true, &discordgo.WebhookParams{
			Content: "Failed to fetch upcoming contests from Codeforces.",
		})
		log.Println("failed to fetch upcoming contests:", err)
		return
	}

	if len(contests) == 0 {
		_, _ = discord.FollowupMessageCreate(interaction.Interaction, true, &discordgo.WebhookParams{
			Content: "No upcoming contests found right now.",
		})
		return
	}

	messages := make([]string, 0, len(contests))
	for _, contest := range contests {
		messages = append(messages, formatDailyAnnouncement(contest))
	}

	for _, chunk := range splitDiscordMessageParts(messages, 1800) {
		msg, err := discord.FollowupMessageCreate(interaction.Interaction, true, &discordgo.WebhookParams{
			Content: chunk,
		})
		if err != nil {
			log.Println("failed to send upcoming contests chunk:", err)
			continue
		}

		storeContestMessage(msg.ID, msg.ChannelID, chunk)
	}
}

func sendAnnouncedContests(discord *discordgo.Session, interaction *discordgo.InteractionCreate) {
	if err := discord.InteractionRespond(interaction.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
	}); err != nil {
		log.Println("failed to defer interaction response:", err)
		return
	}

	sentIDs := loadSentContestIDs()

	contests, err := getContestList()
	if err != nil {
		_, _ = discord.FollowupMessageCreate(interaction.Interaction, true, &discordgo.WebhookParams{
			Content: "Failed to fetch contests from Codeforces.",
		})
		log.Println("failed to fetch contest list:", err)
		return
	}

	announced := make([]string, 0)
	for _, c := range contests {
		if _, ok := sentIDs[c.ID]; ok {
			announced = append(announced, formatContestSummary(c))
		}
	}

	if len(announced) == 0 {
		_, _ = discord.FollowupMessageCreate(interaction.Interaction, true, &discordgo.WebhookParams{
			Content: "No contests have been announced yet for this environment.",
		})
		return
	}

	for _, chunk := range splitDiscordMessageParts(announced, 1800) {
		if _, err := discord.FollowupMessageCreate(interaction.Interaction, true, &discordgo.WebhookParams{
			Content: chunk,
		}); err != nil {
			log.Println("failed to send announced contests chunk:", err)
			continue
		}
	}
}

func getContestList() ([]Info, error) {
	request, err := http.NewRequest(http.MethodGet, "https://codeforces.com/api/contest.list?gym=false", nil)
	if err != nil {
		return nil, err
	}

	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()

	body, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, err
	}

	var apiResponse contestListResponse
	if err := json.Unmarshal(body, &apiResponse); err != nil {
		return nil, err
	}
	if apiResponse.Status != "OK" {
		if apiResponse.Comment != "" {
			return nil, errors.New(apiResponse.Comment)
		}
		return nil, errors.New("codeforces API request failed")
	}

	return apiResponse.Result, nil
}

func loadSentContestIDs() map[int]struct{} {
	if useDB {
		ids, err := loadSentContestIDsDB(environmentName())
		if err != nil {
			log.Println("failed to load sent contest IDs from database:", err)
			return map[int]struct{}{}
		}
		return ids
	}

	sentContestsFileLock.Lock()
	defer sentContestsFileLock.Unlock()

	data, err := os.ReadFile(sentContestsFile)
	if err != nil {
		return map[int]struct{}{}
	}

	var contestIDs []int
	if err := json.Unmarshal(data, &contestIDs); err != nil {
		return map[int]struct{}{}
	}

	result := make(map[int]struct{}, len(contestIDs))
	for _, contestID := range contestIDs {
		result[contestID] = struct{}{}
	}

	return result
}

func saveSentContestIDs(sentContestIDs map[int]struct{}) {
	if useDB {
		if err := saveSentContestIDsDB(sentContestIDs, environmentName()); err != nil {
			log.Println("failed to save sent contest IDs to database:", err)
		}
		return
	}
	contestIDs := make([]int, 0, len(sentContestIDs))
	for contestID := range sentContestIDs {
		contestIDs = append(contestIDs, contestID)
	}
	sort.Ints(contestIDs)

	data, err := json.MarshalIndent(contestIDs, "", "  ")
	if err != nil {
		log.Println("failed to marshal sent contest IDs:", err)
		return
	}

	sentContestsFileLock.Lock()
	defer sentContestsFileLock.Unlock()

	if err := writeAtomic(sentContestsFile, data, 0o644); err != nil {
		log.Println("failed to save sent contest IDs:", err)
	}
}

func writeAtomic(path string, data []byte, perm os.FileMode) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, perm); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func announcementsEnabled() bool {
	if strings.ToLower(strings.TrimSpace(os.Getenv("DISABLE_ANNOUNCEMENTS"))) == "true" {
		return false
	}
	if strings.ToLower(strings.TrimSpace(os.Getenv("CI"))) == "true" {
		return false
	}
	if v := strings.TrimSpace(os.Getenv("ANNOUNCEMENTS_ENABLED")); v != "" {
		return strings.ToLower(v) == "true"
	}
	return true
}

func sendNewContestAnnouncements(discord *discordgo.Session) {
	if !announcementsEnabled() {
		log.Println("Announcements disabled by environment; skipping sending new contest announcements")
		return
	}

	announcementChannel := announcementChannelID()
	if announcementChannel == "" {
		log.Println("DISCORD_ANNOUNCEMENT_CHANNEL_ID is not set; skipping contest announcements")
		return
	}

	contests, err := getContestList()
	if err != nil {
		log.Println("failed to fetch contest list:", err)
		return
	}

	sentContestIDs := loadSentContestIDs()
	now := time.Now().Unix()

	for _, contest := range contests {
		if contest.Phase != "BEFORE" || contest.StartTimeSeconds <= now {
			continue
		}

		if _, alreadySent := sentContestIDs[contest.ID]; alreadySent {
			continue
		}

		msg, err := discord.ChannelMessageSend(announcementChannel, formatDailyAnnouncement(contest))
		if err != nil {
			log.Println("failed to send contest announcement:", err)
			continue
		}

		storeContestMessage(msg.ID, msg.ChannelID, msg.Content)
		sentContestIDs[contest.ID] = struct{}{}
	}

	saveSentContestIDs(sentContestIDs)
}

func startContestWatcher(discord *discordgo.Session) {
	sendNewContestAnnouncements(discord)

	ticker := time.NewTicker(24 * time.Hour)
	go func() {
		defer ticker.Stop()
		for range ticker.C {
			sendNewContestAnnouncements(discord)
		}
	}()
}

func Run() {
	discord, err := discordgo.New("Bot " + BotToken)
	checkNilErr(err)

	env := environmentName()
	log.Printf("starting bot (ENVIRONMENT=%s), announcementsEnabled=%v", env, announcementsEnabled())

	discord.AddHandler(newMessage)
	discord.AddHandler(handleMessageDelete)
	discord.AddHandler(handleReady)
	discord.AddHandler(handleInteractionCreate)

	err = discord.Open()
	checkNilErr(err)
	defer discord.Close()

	fmt.Println("Bot running....")
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)
	<-c
}

func handleInteractionCreate(discord *discordgo.Session, interaction *discordgo.InteractionCreate) {
	if interaction.Type != discordgo.InteractionApplicationCommand {
		return
	}

	data := interaction.ApplicationCommandData()
	switch data.Name {
	case sendAvailableContestCommandName:
		// sendAvailableContest should show contests that have been announced,
		// not trigger new sends. Use the announced-list handler.
		sendAnnouncedContests(discord, interaction)
	case showAllAvailableContestCommandName, showAllAvailbleContestCommandName:
		sendAnnouncedContests(discord, interaction)
	default:
		return
	}
}

func handleMessageDelete(discord *discordgo.Session, messageDelete *discordgo.MessageDelete) {
	contestMessageStoreLock.Lock()
	defer contestMessageStoreLock.Unlock()

	storedMessages := loadContestMessagesLocked()
	storedMessage, ok := storedMessages[messageDelete.ID]
	if !ok {
		return
	}

	msg, err := discord.ChannelMessageSend(storedMessage.ChannelID, storedMessage.Content)
	if err != nil {
		log.Println("failed to resend deleted contest message:", err)
		return
	}

	delete(storedMessages, messageDelete.ID)
	storedMessages[msg.ID] = storedContestMessage{
		ChannelID: msg.ChannelID,
		Content:   storedMessage.Content,
	}
	saveContestMessagesLocked(storedMessages)
}

func newMessage(discord *discordgo.Session, message *discordgo.MessageCreate) {
	if message.Author == nil || message.Author.Bot {
		return
	}

	content := strings.TrimSpace(message.Content)
	if !strings.HasPrefix(content, "!contest") {
		return
	}
	// Use `!contest` to show all contests that have already been announced
	// for this environment. This keeps one consistent command for listing.
	postAnnouncedContestsChannel(discord, message.ChannelID)
}

func postAnnouncedContestsChannel(discord *discordgo.Session, channelID string) {
	sentIDs := loadSentContestIDs()

	contests, err := getContestList()
	if err != nil {
		_, _ = discord.ChannelMessageSend(channelID, "Failed to fetch contests from Codeforces.")
		log.Println("failed to fetch contest list:", err)
		return
	}

	announced := make([]string, 0)
	for _, c := range contests {
		if _, ok := sentIDs[c.ID]; ok {
			announced = append(announced, formatContestSummary(c))
		}
	}

	if len(announced) == 0 {
		_, _ = discord.ChannelMessageSend(channelID, "No contests have been announced yet for this environment.")
		return
	}

	for _, chunk := range splitDiscordMessageParts(announced, 1800) {
		if _, err := discord.ChannelMessageSend(channelID, chunk); err != nil {
			log.Println("failed to send announced contests chunk:", err)
			continue
		}
	}
}
