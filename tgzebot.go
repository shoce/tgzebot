/*

https://pkg.go.dev/github.com/kkdai/youtube/v2/
https://core.telegram.org/bots/api/

GoGet
go get github.com/kkdai/youtube/v2@master

GoFmt
GoBuildNull

*/

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unicode"

	ytdl "github.com/kkdai/youtube/v2"
	"golang.org/x/exp/slices"
	yaml "gopkg.in/yaml.v3"
)

const (
	NL = "\n"

	BEAT = time.Duration(24) * time.Hour / 1000

	TgUpdateLogSize = 108
)

var (
	DEBUG bool

	Interval time.Duration

	YamlConfigPath = "tgzebot.yaml"

	KvToken       string
	KvAccountId   string
	KvNamespaceId string

	TelegramApiUrlBase string = "https://api.telegram.org"

	Ctx context.Context

	HttpClient = &http.Client{}

	YtHttpClient = &http.Client{}
	YtCl         ytdl.Client

	ytRe, ytlistRe *regexp.Regexp

	YtMaxResults int64 = 50
	YtKey        string

	YtHttpClientUserAgent = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/15.2 Safari/605.1.15"

	// https://golang.org/s/re2syntax
	// (?:re)	non-capturing group
	// TODO add support for https://www.youtube.com/watch?&list=PL5Qevr-CpW_yZZjYspehnFc-QRKQMCKHB&v=1nzx7O7ndfI&index=34
	YtReString     = `(?:youtube.com/watch\?v=|youtu.be/|youtube.com/shorts/|youtube.com/live/)([0-9A-Za-z_-]+)`
	YtListReString = `youtube.com/playlist\?list=([0-9A-Za-z_-]+)`

	TgToken     string
	TgUpdateLog []int64
	TgZeChatId  int64

	TgMaxFileSizeBytes int64 = 47 << 20
	TgAudioBitrateKbps int64 = 60

	DownloadLanguages []string

	FfmpegPath          string = "/bin/ffmpeg"
	FfmpegGlobalOptions []string

	TgCommandChannels             string
	TgCommandChannelsPromoteAdmin string

	TgQuest1    string
	TgQuest1Key string
	TgQuest2    string
	TgQuest2Key string
	TgQuest3    string
	TgQuest3Key string

	TgAllChannelsChatIds []int64
)

func init() {
	var err error

	ytRe = regexp.MustCompile(YtReString)
	ytlistRe = regexp.MustCompile(YtListReString)

	if v := os.Getenv("YamlConfigPath"); v != "" {
		YamlConfigPath = v
	}
	if YamlConfigPath == "" {
		log("WARNING YamlConfigPath empty")
	}

	KvToken, _ = GetVar("KvToken")
	if KvToken == "" {
		log("WARNING KvToken empty")
	}

	KvAccountId, _ = GetVar("KvAccountId")
	if KvAccountId == "" {
		log("WARNING KvAccountId empty")
	}

	KvNamespaceId, _ = GetVar("KvNamespaceId")
	if KvNamespaceId == "" {
		log("WARNING KvNamespaceId empty")
	}

	if url, _ := GetVar("TelegramApiUrlBase"); url != "" {
		TelegramApiUrlBase = url
		log("TelegramApiUrlBase:`%s`", TelegramApiUrlBase)
	}

	if s, _ := GetVar("TgMaxFileSizeBytes"); s != "" {
		if v, err := strconv.ParseInt(s, 10, 64); err == nil {
			TgMaxFileSizeBytes = v
			log("TgMaxFileSizeBytes:%v", TgMaxFileSizeBytes)
		} else {
			log("WARNING invalid TgMaxFileSizeBytes:`%s`", s)
		}
	}

	if s, _ := GetVar("TgAudioBitrateKbps"); s != "" {
		if v, err := strconv.ParseInt(s, 10, 64); err == nil {
			TgAudioBitrateKbps = v
			log("TgAudioBitrateKbps:%v", TgAudioBitrateKbps)
		} else {
			log("WARNING invalid TgAudioBitrateKbps:`%s`", s)
		}
	}

	Ctx = context.TODO()

	YtProxy := http.ProxyFromEnvironment
	if v, _ := GetVar("YtProxyList"); v != "" {
		pp := strings.Split(v, " ")
		rand.Seed(time.Now().UnixNano())
		if err := SetVar("YtProxy", pp[rand.Intn(len(pp))]); err != nil {
			log("WARNING SetVar YtProxy: %v", err)
		}
	}
	YtProxyUrl, _ := GetVar("YtProxy")
	if YtProxyUrl != "" {
		if !strings.HasPrefix(YtProxyUrl, "https://") {
			YtProxyUrl = "https://" + YtProxyUrl
		}
		log("YtProxy: %s", YtProxyUrl)
		if YtProxyUrl, err := url.Parse(YtProxyUrl); err == nil {
			YtProxy = http.ProxyURL(YtProxyUrl)
		}
	}

	var proxyTransport http.RoundTripper = http.DefaultTransport
	proxyTransport.(*http.Transport).Proxy = YtProxy
	YtCl = ytdl.Client{HTTPClient: &http.Client{Transport: &UserAgentTransport{proxyTransport, YtHttpClientUserAgent}}}

	if s, _ := GetVar("Interval"); s != "" {
		Interval, err = time.ParseDuration(s)
		if err != nil {
			log("ERROR time.ParseDuration Interval:`%s`: %v", s, err)
			os.Exit(1)
		}
		log("Interval: %v", Interval)
	} else {
		log("ERROR Interval empty")
		os.Exit(1)
	}

	TgToken, _ = GetVar("TgToken")
	if TgToken == "" {
		log("ERROR TgToken empty")
		os.Exit(1)
	}

	if v, err := GetVar("TgUpdateLog"); err != nil {
		log("ERROR GetVar TgUpdateLog: %w", err)
		os.Exit(1)
	} else {
		for _, s := range strings.Split(v, " ") {
			if s == "" {
				continue
			}
			if i, err := strconv.ParseInt(s, 10, 0); err != nil {
				log("WARNING %v", err)
				continue
			} else {
				TgUpdateLog = append(TgUpdateLog, i)
			}
		}
	}
	log("DEBUG TgUpdateLog: %+v", TgUpdateLog)

	if v, _ := GetVar("TgZeChatId"); v == "" {
		log("ERROR TgZeChatId empty")
		os.Exit(1)
	} else {
		TgZeChatId, err = strconv.ParseInt(v, 10, 0)
		if err != nil {
			log("ERROR invalid TgZeChatId: %v", err)
			os.Exit(1)
		}
	}

	TgCommandChannels, _ = GetVar("TgCommandChannels")
	if TgCommandChannels == "" {
		log("ERROR TgCommandChannels empty")
		os.Exit(1)
	}

	TgCommandChannelsPromoteAdmin, _ = GetVar("TgCommandChannelsPromoteAdmin")
	if TgCommandChannelsPromoteAdmin == "" {
		log("ERROR TgCommandChannelsPromoteAdmin empty")
		os.Exit(1)
	}

	TgQuest1, _ = GetVar("TgQuest1")
	TgQuest1Key, _ = GetVar("TgQuest1Key")

	TgQuest2, _ = GetVar("TgQuest2")
	TgQuest2Key, _ = GetVar("TgQuest2Key")

	TgQuest3, _ = GetVar("TgQuest3")
	TgQuest3Key, _ = GetVar("TgQuest3Key")

	YtKey, _ = GetVar("YtKey")
	if YtKey == "" {
		log("ERROR YtKey empty")
		os.Exit(1)
	}

	if v, _ := GetVar("YtMaxResults"); v != "" {
		YtMaxResults, err = strconv.ParseInt(v, 10, 0)
		if err != nil {
			log("ERROR invalid YtMaxResults: %v", err)
			os.Exit(1)
		}
	}

	if v, _ := GetVar("YtHttpClientUserAgent"); v != "" {
		YtHttpClientUserAgent = v
	}
	if v, _ := GetVar("YtReString"); v != "" {
		YtReString = v
	}
	if v, _ := GetVar("YtListReString"); v != "" {
		YtListReString = v
	}

	if v, _ := GetVar("FfmpegPath"); v != "" {
		FfmpegPath = v
	}
	log("FfmpegPath:`%s`", FfmpegPath)

	if v, _ := GetVar("FfmpegGlobalOptions"); v != "" {
		FfmpegGlobalOptions = strings.Split(v, " ")
	}

	if v, _ := GetVar("DownloadLanguages"); v != "" {
		DownloadLanguages = strings.Split(v, " ")
	}

	if v, err := GetVar("TgAllChannelsChatIds"); err != nil {
		log("ERROR GetVar TgAllChannelsChatIds: %w", err)
		os.Exit(1)
	} else {
		for _, s := range strings.Split(v, " ") {
			if i, err := strconv.ParseInt(s, 10, 0); err != nil {
				log("WARNING invalid TgAllChannelsChatIds: %v", err)
				continue
			} else {
				TgAllChannelsChatIds = append(TgAllChannelsChatIds, i)
			}
		}
	}
}

func main() {
	sigterm := make(chan os.Signal, 1)
	signal.Notify(sigterm, syscall.SIGTERM)
	go func(sigterm chan os.Signal) {
		<-sigterm
		tgsendMessage(fmt.Sprintf("%s: sigterm", os.Args[0]), TgZeChatId, "", 0)
		log("sigterm received")
		os.Exit(1)
	}(sigterm)

	for {
		t0 := time.Now()
		processTgUpdates()
		if dur := time.Now().Sub(t0); dur < Interval {
			time.Sleep(Interval - dur)
		}
	}

	return
}

func beats(td time.Duration) int {
	return int(td / BEAT)
}

func ts() string {
	t := time.Now().Local()
	return fmt.Sprintf(
		"%03d."+"%02d%02d."+"%02d%02d",
		t.Year()%1000, t.Month(), t.Day(), t.Hour(), t.Minute(),
	)
}

func log(msg interface{}, args ...interface{}) {
	msgtext := fmt.Sprintf("%s %s", ts(), msg) + NL
	fmt.Fprintf(os.Stderr, msgtext, args...)
}

type TgChatMessageId struct {
	ChatId    int64
	MessageId int64
}

type YtChannel struct {
	Id             string `json:"id"`
	ContentDetails struct {
		RelatedPlaylists struct {
			Uploads string `json:"uploads"`
		} `json:"relatedPlaylists"`
	} `json:"contentDetails"`
}

type YtChannelListResponse struct {
	PageInfo struct {
		TotalResults   int64 `json:"totalResults"`
		ResultsPerPage int64 `json:"resultsPerPage"`
	} `json:"pageInfo"`
	Items []YtChannel `json:"items"`
}

type YtPlaylistSnippet struct {
	Title        string `json:"title"`
	Description  string `json:"description"`
	PublishedAt  string `json:"publishedAt"`
	ChannelId    string `json:"channelId"`
	ChannelTitle string `json:"channelTitle"`
	Thumbnails   struct {
		Medium struct {
			Url string `json:"url"`
		} `json:"medium"`
		High struct {
			Url string `json:"url"`
		} `json:"high"`
		Standard struct {
			Url string `json:"url"`
		} `json:"standard"`
		MaxRes struct {
			Url string `json:"url"`
		} `json:"maxres"`
	} `json:"thumbnails"`
}

type YtPlaylist struct {
	Snippet        YtPlaylistSnippet `json:"snippet"`
	ContentDetails struct {
		ItemCount uint `json:"itemCount"`
	} `json:"contentDetails"`
}

type YtPlaylists struct {
	NextPageToken string `json:"nextPageToken"`
	PageInfo      struct {
		TotalResults   int64 `json:"totalResults"`
		ResultsPerPage int64 `json:"resultsPerPage"`
	} `json:"pageInfo"`
	Items []YtPlaylist
}

type YtPlaylistItemSnippet struct {
	Title        string `json:"title"`
	Description  string `json:"description"`
	PublishedAt  string `json:"publishedAt"`
	ChannelId    string `json:"channelId"`
	ChannelTitle string `json:"channelTitle"`
	PlaylistId   string `json:"playlistId"`
	Thumbnails   struct {
		Medium struct {
			Url string `json:"url"`
		} `json:"medium"`
		High struct {
			Url string `json:"url"`
		} `json:"high"`
		Standard struct {
			Url string `json:"url"`
		} `json:"standard"`
		MaxRes struct {
			Url string `json:"url"`
		} `json:"maxres"`
	} `json:"thumbnails"`
	Position   int64 `json:"position"`
	ResourceId struct {
		VideoId string `json:"videoId"`
	} `json:"resourceId"`
}

type YtPlaylistItem struct {
	Snippet YtPlaylistItemSnippet `json:"snippet"`
}

type YtPlaylistItems struct {
	NextPageToken string `json:"nextPageToken"`
	PageInfo      struct {
		TotalResults   int64 `json:"totalResults"`
		ResultsPerPage int64 `json:"resultsPerPage"`
	} `json:"pageInfo"`
	Items []YtPlaylistItem
}

type TgResponse struct {
	Ok          bool       `json:"ok"`
	Description string     `json:"description"`
	Result      *TgMessage `json:"result"`
}

type TgResponseShort struct {
	Ok          bool   `json:"ok"`
	Description string `json:"description"`
}

type TgPromoteChatMemberResponse struct {
	Ok          bool   `json:"ok"`
	Description string `json:"description"`
	Result      bool   `json:"result"`
}

type TgPhotoSize struct {
	FileId       string `json:"file_id"`
	FileUniqueId string `json:"file_unique_id"`
	Width        int64  `json:"width"`
	Height       int64  `json:"height"`
	FileSize     int64  `json:"file_size"`
}

type TgAudio struct {
	FileId       string      `json:"file_id"`
	FileUniqueId string      `json:"file_unique_id"`
	Duration     int64       `json:"duration"`
	Performer    string      `json:"performer"`
	Title        string      `json:"title"`
	MimeType     string      `json:"mime_type"`
	FileSize     int64       `json:"file_size"`
	Thumb        TgPhotoSize `json:"thumb"`
}

type TgVideo struct {
	FileId       string      `json:"file_id"`
	FileUniqueId string      `json:"file_unique_id"`
	Width        int64       `json:"width"`
	Height       int64       `json:"height"`
	Duration     int64       `json:"duration"`
	MimeType     string      `json:"mime_type"`
	FileSize     int64       `json:"file_size"`
	Thumb        TgPhotoSize `json:"thumb"`
}

type TgMessage struct {
	MessageId int64  `json:"message_id"`
	From      TgUser `json:"from"`
	Chat      TgChat `json:"chat"`
	Text      string
	Audio     TgAudio       `json:"audio"`
	Photo     []TgPhotoSize `json:"photo"`
	Video     TgVideo       `json:"video"`
}

type TgUser struct {
	Id        int64  `json:"id"`
	IsBot     bool   `json:"is_bot"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
	Username  string `json:"username"`
}

type TgChat struct {
	Id         int64  `json:"id"`
	Type       string `json:"type"`
	Title      string `json:"title"`
	Username   string `json:"username"`
	FirstName  string `json:"first_name"`
	LastName   string `json:"last_name"`
	InviteLink string `json:"invite_link"`
}

type TgGetUpdatesResponse struct {
	Ok          bool       `json:"ok"`
	Description string     `json:"description"`
	Result      []TgUpdate `json:"result"`
}

type TgChatMemberUpdated struct {
	Chat                    TgChat       `json:"chat"`
	From                    TgUser       `json:"from"`
	Date                    int64        `json:"date"`
	OldChatMember           TgChatMember `json:"old_chat_member"`
	NewChatMember           TgChatMember `json:"new_chat_member"`
	ViaJoinRequest          bool         `json:"via_join_request"`
	ViaChatFolderInviteLink bool         `json:"via_chat_folder_invite_link"`
}

type TgUpdate struct {
	UpdateId            int64               `json:"update_id"`
	Message             TgMessage           `json:"message"`
	EditedMessage       TgMessage           `json:"edited_message"`
	ChannelPost         TgMessage           `json:"channel_post"`
	EditedChannelPost   TgMessage           `json:"edited_channel_post"`
	MyChatMemberUpdated TgChatMemberUpdated `json:"my_chat_member"`
}

type TgGetChatResponse struct {
	Ok          bool   `json:"ok"`
	Description string `json:"description"`
	Result      TgChat `json:"result"`
}

type TgChatMember struct {
	User   TgUser `json:"user"`
	Status string `json:"status"`
}

type TgGetChatAdministratorsRequest struct {
	ChatId string `json:"chat_id"`
}

type TgGetChatAdministratorsResponse struct {
	Ok          bool           `json:"ok"`
	Description string         `json:"description"`
	Result      []TgChatMember `json:"result"`
}

type YtVideo struct {
	Id            string
	PlaylistId    string
	PlaylistIndex int64
	PlaylistSize  int64
	PlaylistTitle string
}

type UserAgentTransport struct {
	T     http.RoundTripper
	Agent string
}

func (uat *UserAgentTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req.Header.Set("User-Agent", uat.Agent)
	return uat.T.RoundTrip(req)
}

func getJson(url string, target interface{}, respjson *string) (err error) {
	resp, err := HttpClient.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	var respBody []byte
	respBody, err = io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("io.ReadAll: %w", err)
	}

	err = json.NewDecoder(bytes.NewBuffer(respBody)).Decode(target)
	if err != nil {
		return fmt.Errorf("json.Decoder.Decode: %w", err)
	}

	if DEBUG {
		log("DEBUG getJson %s response ContentLength:%d Body:"+NL+"%s", url, resp.ContentLength, respBody)
	}
	if respjson != nil {
		*respjson = string(respBody)
	}

	return nil
}

func postJson(url string, data *bytes.Buffer, target interface{}) error {
	resp, err := HttpClient.Post(
		url,
		"application/json",
		data,
	)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	var respBody []byte
	respBody, err = io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("io.ReadAll: %w", err)
	}

	err = json.NewDecoder(bytes.NewBuffer(respBody)).Decode(target)
	if err != nil {
		return fmt.Errorf("Decode: %w", err)
	}

	return nil
}

func GetVar(name string) (value string, err error) {
	if DEBUG {
		log("DEBUG GetVar `%s`", name)
	}

	value = os.Getenv(name)

	if YamlConfigPath != "" {
		if v, err := YamlGet(name); err != nil {
			log("WARNING GetVar YamlGet `%s`: %w", name, err)
			return "", err
		} else if v != "" {
			value = v
		}
	}

	if KvToken != "" && KvAccountId != "" && KvNamespaceId != "" {
		if v, err := KvGet(name); err != nil {
			log("WARNING GetVar KvGet `%s`: %w", name, err)
			return "", err
		} else if v != "" {
			value = v
		}
	}

	return value, nil
}

func SetVar(name, value string) (err error) {
	if DEBUG {
		log("DEBUG SetVar: %s: %s", name, value)
	}

	if KvToken != "" && KvAccountId != "" && KvNamespaceId != "" {
		err = KvSet(name, value)
		if err != nil {
			return err
		}
		return nil
	}

	if YamlConfigPath != "" {
		err = YamlSet(name, value)
		if err != nil {
			return err
		}
		return nil
	}

	return fmt.Errorf("not kv credentials nor yaml config path provided to save to")
}

func YamlGet(name string) (value string, err error) {
	configf, err := os.Open(YamlConfigPath)
	if err != nil {
		//log("WARNING os.Open config file %s: %v", YamlConfigPath, err)
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	defer configf.Close()

	configm := make(map[interface{}]interface{})
	if err = yaml.NewDecoder(configf).Decode(&configm); err != nil {
		//log("WARNING yaml.Decode %s: %v", YamlConfigPath, err)
		return "", err
	}

	if v, ok := configm[name]; ok == true {
		switch v.(type) {
		case string:
			value = v.(string)
		case int:
			value = fmt.Sprintf("%d", v.(int))
		default:
			return "", fmt.Errorf("yaml value of unsupported type, only string and int types are supported")
		}
	}

	return value, nil
}

func YamlSet(name, value string) error {
	configf, err := os.Open(YamlConfigPath)
	if err == nil {
		configm := make(map[interface{}]interface{})
		err := yaml.NewDecoder(configf).Decode(&configm)
		if err != nil {
			log("WARNING yaml.Decode %s: %v", YamlConfigPath, err)
		}
		configf.Close()
		configm[name] = value
		configf, err := os.Create(YamlConfigPath)
		if err == nil {
			defer configf.Close()
			confige := yaml.NewEncoder(configf)
			err := confige.Encode(configm)
			if err == nil {
				confige.Close()
				configf.Close()
			} else {
				log("WARNING yaml.Encoder.Encode: %v", err)
				return err
			}
		} else {
			log("WARNING os.Create config file %s: %v", YamlConfigPath, err)
			return err
		}
	} else {
		log("WARNING os.Open config file %s: %v", YamlConfigPath, err)
		return err
	}

	return nil
}

func KvGet(name string) (value string, err error) {
	req, err := http.NewRequest(
		"GET",
		fmt.Sprintf("https://api.cloudflare.com/client/v4/accounts/%s/storage/kv/namespaces/%s/values/%s", KvAccountId, KvNamespaceId, name),
		nil,
	)
	if err != nil {
		return "", err
	}

	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", KvToken))
	resp, err := HttpClient.Do(req)
	if err != nil {
		return "", err
	}
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("kv api response status: %s", resp.Status)
	}

	if rbb, err := io.ReadAll(resp.Body); err != nil {
		return "", err
	} else {
		value = string(rbb)
	}

	return value, nil
}

func KvSet(name, value string) error {
	mpbb := new(bytes.Buffer)
	mpw := multipart.NewWriter(mpbb)
	if err := mpw.WriteField("metadata", "{}"); err != nil {
		return err
	}
	if err := mpw.WriteField("value", value); err != nil {
		return err
	}
	mpw.Close()

	req, err := http.NewRequest(
		"PUT",
		fmt.Sprintf("https://api.cloudflare.com/client/v4/accounts/%s/storage/kv/namespaces/%s/values/%s", KvAccountId, KvNamespaceId, name),
		mpbb,
	)
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", mpw.FormDataContentType())
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", KvToken))
	resp, err := HttpClient.Do(req)
	if err != nil {
		return err
	}
	if resp.StatusCode != 200 {
		return fmt.Errorf("kv api response status: %s", resp.Status)
	}

	return nil
}

type KvKeysResponse struct {
	Result []struct {
		Name     string            `json:"name"`
		Metadata map[string]string `json:"metadata"`
	} `json:"result"`
	Success    bool     `json:"success"`
	Errors     []string `json:"errors"`
	Messages   []string `json:"messages"`
	ResultInfo struct {
		Count  int
		Cursor string
	} `json:"result_info"`
}

func KvKeys() (kvkeys *KvKeysResponse, err error) {
	req, err := http.NewRequest(
		"GET",
		fmt.Sprintf("https://api.cloudflare.com/client/v4/accounts/%s/storage/kv/namespaces/%s/keys", KvAccountId, KvNamespaceId),
		nil,
	)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", KvToken))
	resp, err := HttpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("kv api response status: %s", resp.Status)
	}

	kvkeys = new(KvKeysResponse)
	err = json.NewDecoder(resp.Body).Decode(kvkeys)
	if err != nil {
		return nil, fmt.Errorf("Decode: %w", err)
	}

	return kvkeys, nil
}

func tgescape(text string) string {
	// https://core.telegram.org/bots/api#markdownv2-style
	return strings.NewReplacer(
		"`", "\\`",
		".", "\\.",
		"-", "\\-",
		"_", "\\_",
		"#", "\\#",
		"*", "\\*",
		"~", "\\~",
		">", "\\>",
		"+", "\\+",
		"=", "\\=",
		"|", "\\|",
		"!", "\\!",
		"{", "\\{",
		"}", "\\}",
		"[", "\\[",
		"]", "\\]",
		"(", "\\(",
		")", "\\)",
	).Replace(text)
}

func tggetUpdates() (uu []TgUpdate, tgrespjson string, err error) {

	/*
		var offset int64
		if len(TgUpdateLog) > 0 {
			offset = TgUpdateLog[len(TgUpdateLog)-1] + 1
		}
		getUpdatesUrl := fmt.Sprintf("%s/bot%s/getUpdates?offset=%d", TelegramApiUrlBase, TgToken, offset)
	*/

	getUpdatesUrl := fmt.Sprintf("%s/bot%s/getUpdates", TelegramApiUrlBase, TgToken)
	var tgResp TgGetUpdatesResponse

	err = getJson(getUpdatesUrl, &tgResp, &tgrespjson)
	if err != nil {
		return nil, "", err
	}
	if !tgResp.Ok {
		return nil, "", fmt.Errorf("Tg response not ok: %s", tgResp.Description)
	}

	return tgResp.Result, tgrespjson, nil
}

func tggetChat(chatid int64) (chat TgChat, err error) {
	getChatUrl := fmt.Sprintf("%s/bot%s/getChat?chat_id=%d", TelegramApiUrlBase, TgToken, chatid)
	var tgResp TgGetChatResponse

	tries := []int{1, 2, 3}
	for ti, _ := range tries {
		err = getJson(getChatUrl, &tgResp, nil)
		if err != nil {
			return TgChat{}, err
		}
		if !tgResp.Ok {
			if strings.HasPrefix(tgResp.Description, "Too Many Requests: retry after ") && ti < len(tries)-1 {
				log("tggetChat %d: Tg: %s: sleeping 17 seconds", chatid, tgResp.Description)
				time.Sleep(17 * time.Second)
				continue
			}
			return TgChat{}, fmt.Errorf("Tg response not ok: %s", tgResp.Description)
		}
	}

	return tgResp.Result, nil
}

func tgpromoteChatMember(chatid, userid int64) (bool, error) {
	// https://core.telegram.org/bots/api#promotechatmember
	promoteChatMember := map[string]interface{}{
		"chat_id":                chatid,
		"user_id":                userid,
		"is_anonymous":           false,
		"can_manage_chat":        true,
		"can_post_messages":      true,
		"can_edit_messages":      true,
		"can_delete_messages":    true,
		"can_change_info":        true,
		"can_restrict_members":   true,
		"can_promote_members":    true,
		"can_invite_users":       true,
		"can_manage_voice_chats": true,
	}
	promoteChatMemberJSON, err := json.Marshal(promoteChatMember)
	if err != nil {
		return false, err
	}

	var tgresp TgPromoteChatMemberResponse
	err = postJson(
		fmt.Sprintf("%s/bot%s/promoteChatMember", TelegramApiUrlBase, TgToken),
		bytes.NewBuffer(promoteChatMemberJSON),
		&tgresp,
	)
	if err != nil {
		return false, fmt.Errorf("postJson: %w", err)
	}

	if !tgresp.Ok {
		return false, fmt.Errorf("%s", tgresp.Description)
	}

	return tgresp.Result, nil
}

func tggetChatAdministrators(chatid int64) (mm []TgChatMember, err error) {
	getChatAdministratorsUrl := fmt.Sprintf("%s/bot%s/getChatAdministrators?chat_id=%d", TelegramApiUrlBase, TgToken, chatid)
	var tgResp TgGetChatAdministratorsResponse

	err = getJson(getChatAdministratorsUrl, &tgResp, nil)
	if err != nil {
		return nil, err
	}
	if !tgResp.Ok {
		return nil, fmt.Errorf("Tg response not ok: %s", tgResp.Description)
	}

	return tgResp.Result, nil
}

func processTgUpdates() {
	var err error

	var tgdeleteMessages []TgChatMessageId
	defer func(mm *[]TgChatMessageId) {
		for _, cm := range *mm {
			tgdeleteMessage(cm.ChatId, cm.MessageId)
		}
	}(&tgdeleteMessages)

	var uu []TgUpdate
	var respjson string
	uu, respjson, err = tggetUpdates()
	if err != nil {
		log("tggetUpdates: %v", err)
		os.Exit(1)
	}

	var m, prevm TgMessage
	for _, u := range uu {

		log("#")

		/*
			if len(TgUpdateLog) > 0 && u.UpdateId < TgUpdateLog[len(TgUpdateLog)-1] {
				log("WARNING this telegram update id:%d is older than last id:%d, skipping", u.UpdateId, TgUpdateLog[len(TgUpdateLog)-1])
				continue
			}
		*/

		if slices.Contains(TgUpdateLog, u.UpdateId) {
			log("WARNING this telegram update id:%d was already processed, skipping", u.UpdateId)
			continue
		}

		TgUpdateLog = append(TgUpdateLog, u.UpdateId)
		if len(TgUpdateLog) > TgUpdateLogSize {
			TgUpdateLog = TgUpdateLog[len(TgUpdateLog)-TgUpdateLogSize:]
		}

		TgUpdateLogString := []string{}
		for _, i := range TgUpdateLog {
			TgUpdateLogString = append(TgUpdateLogString, fmt.Sprintf("%d", i))
		}
		if err := SetVar("TgUpdateLog", strings.Join(TgUpdateLogString, " ")); err != nil {
			log("WARNING SetVar TgUpdateLog: %v", err)
		}

		var iseditmessage bool
		var ischannelpost bool
		if u.Message.MessageId != 0 {
			m = u.Message
		} else if u.EditedMessage.MessageId != 0 {
			m = u.EditedMessage
			iseditmessage = true
		} else if u.ChannelPost.MessageId != 0 {
			m = u.ChannelPost
			ischannelpost = true
		} else if u.EditedChannelPost.MessageId != 0 {
			m = u.EditedChannelPost
			ischannelpost = true
			iseditmessage = true
		} else if u.MyChatMemberUpdated.Date != 0 {
			cmu := u.MyChatMemberUpdated
			report := fmt.Sprintf(
				"*MyChatMemberUpdated*"+NL+
					"from:"+NL+
					"  username: @%s"+NL+
					"  id: `%d`"+NL+
					"chat:"+NL+
					"  id: `%d`"+NL+
					"  username: @%s"+NL+
					"  type: %s"+NL+
					"  title: %s"+NL+
					"old member:"+NL+
					"  username: @%s"+NL+
					"  id: `%d`"+NL+
					"  status: %s"+NL+
					"new member:"+NL+
					"  username: @%s"+NL+
					"  id: `%d`"+NL+
					"  status: %s"+NL+
					"",
				cmu.From.Username, cmu.From.Id,
				cmu.Chat.Id, cmu.Chat.Username, cmu.Chat.Type, tgescape(cmu.Chat.Title),
				cmu.OldChatMember.User.Username, cmu.OldChatMember.User.Id, cmu.OldChatMember.Status,
				cmu.NewChatMember.User.Username, cmu.NewChatMember.User.Id, cmu.NewChatMember.Status,
			)
			_, err = tgsendMessage(report, TgZeChatId, "MarkdownV2", 0)
			if err != nil {
				log("tgsendMessage: %w", err)
			}
		} else {
			log("WARNING unsupported type of update id:%d received:"+NL+"%s", u.UpdateId, respjson)
			_, err = tgsendMessage(fmt.Sprintf("unsupported type of update (id:%d) received:"+NL+"```"+NL+"%s"+NL+"```", u.UpdateId, respjson), TgZeChatId, "MarkdownV2", 0)
			if err != nil {
				log("WARNING tgsendMessage: %w", err)
				continue
			}
			continue
		}

		if m.Chat.Type == "channel" {
			ischannelpost = true
		}

		if ischannelpost {
			add := true
			for _, i := range TgAllChannelsChatIds {
				if m.Chat.Id == i {
					add = false
				}
			}
			if add {
				TgAllChannelsChatIds = append(TgAllChannelsChatIds, m.Chat.Id)
			}

			sort.Slice(TgAllChannelsChatIds, func(i, j int) bool { return TgAllChannelsChatIds[i] < TgAllChannelsChatIds[j] })
			ss := []string{}
			for _, i := range TgAllChannelsChatIds {
				ss = append(ss, fmt.Sprintf("%d", i))
			}
			s := strings.Join(ss, " ")

			if v, err := GetVar("TgAllChannelsChatIds"); err != nil {
				log("ERROR GetVar TgAllChannelsChatIds: %w", err)
				continue
			} else if v != s {
				if err := SetVar("TgAllChannelsChatIds", s); err != nil {
					log("WARNING SetVar TgAllChannelsChatIds: %w", err)
				}
			}
		}

		log("telegram message from:`%s` chat:`%s` text:`%s`", m.From.Username, m.Chat.Username, m.Text)
		if m.Text == "" {
			continue
		}

		shouldreport := true
		if m.From.Id == TgZeChatId {
			shouldreport = false
		}
		var chatadmins string
		if aa, err := tggetChatAdministrators(m.Chat.Id); err == nil {
			for _, a := range aa {
				chatadmins += fmt.Sprintf("username:@%s id:%d status:%s  ", a.User.Username, a.User.Id, a.Status)
				if a.User.Id == TgZeChatId {
					shouldreport = false
				}
			}
		} else {
			log("tggetChatAdministrators: %v", err)
		}
		if shouldreport && m.MessageId != 0 {
			report := fmt.Sprintf(
				"*Message*"+NL+
					"from: username:@%s id:`%d`"+NL+
					"chat: username:@%s id:%d type:%s title:%s"+NL+
					"chat admins: %s"+NL+
					"iseditmessage:%v"+NL+
					"text:"+NL+
					"```"+NL+
					"%s"+NL+
					"```",
				m.From.Username, m.From.Id,
				m.Chat.Id, m.Chat.Username, m.Chat.Type, tgescape(m.Chat.Title),
				chatadmins,
				iseditmessage,
				m.Text,
			)
			_, err = tgsendMessage(report, TgZeChatId, "MarkdownV2", 0)
			if err != nil {
				log("tgsendMessage: %v", err)
				continue
			}
		}

		if strings.TrimSpace(m.Text) == "/id" {
			_, err = tgsendMessage(
				fmt.Sprintf("username `%s`"+NL+"user id `%d`"+NL+"chat id `%d`", m.From.Username, m.From.Id, m.Chat.Id),
				m.Chat.Id, "MarkdownV2", m.MessageId,
			)
			if err != nil {
				log("tgsendMessage: %v", err)
			}
		}

		if strings.TrimSpace(m.Text) == TgCommandChannels {
			var totalchannels, removedchannels int
			totalchannels = len(TgAllChannelsChatIds)
			for _, i := range TgAllChannelsChatIds {
				var err error
				c, getChatErr := tggetChat(i)
				if getChatErr != nil {
					if strings.Contains(getChatErr.Error(), "Bad Request: chat not found") {
						// Remove the channel
						removedchannels += 1
						continue
					}
					_, err = tgsendMessage(fmt.Sprintf("id:%d err:%v", i, getChatErr), m.Chat.Id, "", 0)
					if err != nil {
						log("tgsendMessage: %v", err)
					}
					continue
				}
				chatinfo := c.Title
				if c.Username != "" {
					chatinfo += " " + fmt.Sprintf("https://t.me/%s", c.Username)
				} else if c.InviteLink != "" {
					chatinfo += " " + c.InviteLink
				}
				_, err = tgsendMessage(chatinfo, m.Chat.Id, "", 0)
				if err != nil {
					log("tgsendMessage: %v", err)
				}
			}
			totalmessage := fmt.Sprintf("Total %d channels.", totalchannels)
			if removedchannels > 0 {
				totalmessage += NL + fmt.Sprintf("Removed %d channels.", removedchannels)
			}
			_, err = tgsendMessage(totalmessage, m.Chat.Id, "", m.MessageId)
			if err != nil {
				log("tgsendMessage: %v", err)
			}
		}

		if strings.TrimSpace(m.Text) == TgCommandChannelsPromoteAdmin {
			var total, totalok int
			for _, i := range TgAllChannelsChatIds {
				success, err := tgpromoteChatMember(i, m.From.Id)
				total++
				if success != true || err != nil {
					log("tgpromoteChatMember %d %d: %v", i, m.From.Id, err)
				} else {
					totalok++
					log("tgpromoteChatMember %d %d: ok", i, m.From.Id)
				}
			}
			_, err = tgsendMessage(fmt.Sprintf("ok for %d of total %d channels.", totalok, total), m.Chat.Id, "", m.MessageId)
			if err != nil {
				log("tgsendMessage: %v", err)
			}
		}

		if strings.TrimSpace(m.Text) == TgQuest1 {
			_, err = tgsendMessage(TgQuest1Key, m.Chat.Id, "", 0)
			if err != nil {
				log("tgsendMessage: %v", err)
			}
		}
		if strings.TrimSpace(m.Text) == TgQuest2 {
			_, err = tgsendMessage(TgQuest2Key, m.Chat.Id, "", 0)
			if err != nil {
				log("tgsendMessage: %v", err)
			}
		}
		if strings.TrimSpace(m.Text) == TgQuest3 {
			_, err = tgsendMessage(TgQuest3Key, m.Chat.Id, "", 0)
			if err != nil {
				log("tgsendMessage: %v", err)
			}
		}

		var downloadvideo bool
		if strings.HasPrefix(strings.ToLower(m.Text), "video ") || strings.HasSuffix(strings.ToLower(m.Text), " video") || strings.ToLower(prevm.Text) == "video" || strings.HasPrefix(strings.ToLower(m.Chat.Title), "vi") {
			downloadvideo = true
		}
		prevm = m

		var videos []YtVideo

		if mm := ytlistRe.FindStringSubmatch(m.Text); len(mm) > 1 {
			videos, err = getList(mm[1])
			if err != nil {
				log("getList: %v", err)
				continue
			}
		} else if mm := ytRe.FindStringSubmatch(m.Text); len(mm) > 1 {
			videos = []YtVideo{YtVideo{Id: mm[1]}}
		}

		if len(videos) > 0 {

			var postingerr error
			var vinfo *ytdl.Video
			for _, v := range videos {
				vinfo, err = YtCl.GetVideoContext(Ctx, v.Id)
				if err != nil {
					log("ERROR GetVideoContext: %v", err)
					postingerr = err
					break
				}

				if downloadvideo {
					err = postVideo(v, vinfo, m)
					if err != nil {
						log("ERROR postVideo: %v", err)
						postingerr = err
						break
					}
				} else {
					err = postAudio(v, vinfo, m)
					if err != nil {
						log("ERROR postAudio: %v", err)
						postingerr = err
						break
					}
				}

				if len(videos) > 3 {
					time.Sleep(11 * time.Second)
				}
			}

			if postingerr == nil {
				if ischannelpost {
					err = tgdeleteMessage(m.Chat.Id, m.MessageId)
					if err != nil {
						log("tgdeleteMessage: %v", err)
					}
				}
			} else {
				_, err = tgsendMessage(fmt.Sprintf("ERROR %v", postingerr), m.Chat.Id, "", m.MessageId)
				if err != nil {
					log("tgsendMessage: %v", err)
				}
			}

		}
	}

	return
}

func postVideo(v YtVideo, vinfo *ytdl.Video, m TgMessage) error {
	var videoFormat, videoSmallestFormat ytdl.Format

	var tgdeleteMessages []TgChatMessageId
	defer func(mm *[]TgChatMessageId) {
		for _, cm := range *mm {
			tgdeleteMessage(cm.ChatId, cm.MessageId)
		}
	}(&tgdeleteMessages)

	for _, f := range vinfo.Formats.WithAudioChannels() {
		if !strings.Contains(f.MimeType, "/mp4") {
			continue
		}
		fsize := f.ContentLength
		if fsize == 0 {
			fsize = int64(f.Bitrate / 8 * int(vinfo.Duration.Seconds()))
		}
		if !strings.HasPrefix(f.MimeType, "video/mp4") || f.QualityLabel == "" || f.AudioQuality == "" {
			continue
		}
		flang := strings.ToLower(f.LanguageDisplayName())
		log("format: ContentLength:%dmb Language:%#v", f.ContentLength>>20, flang)
		if flang != "" {
			skip := true
			for _, l := range DownloadLanguages {
				if strings.Contains(flang, l) {
					skip = false
				}
			}
			if skip {
				continue
			}
		}
		if videoSmallestFormat.ItagNo == 0 || f.Bitrate < videoSmallestFormat.Bitrate {
			videoSmallestFormat = f
		}
		if fsize < TgMaxFileSizeBytes && f.Bitrate > videoFormat.Bitrate {
			videoFormat = f
		}
	}

	var targetVideoBitrateKbps int64
	if videoFormat.ItagNo == 0 {
		videoFormat = videoSmallestFormat
		targetVideoSize := int64(TgMaxFileSizeBytes - (TgAudioBitrateKbps*1024*int64(vinfo.Duration.Seconds()+1))/8)
		targetVideoBitrateKbps = int64(((targetVideoSize * 8) / int64(vinfo.Duration.Seconds()+1)) / 1024)
	}

	ytstream, ytstreamsize, err := YtCl.GetStreamContext(Ctx, vinfo, &videoFormat)
	if err != nil {
		return fmt.Errorf("GetStreamContext: %w", err)
	}
	defer ytstream.Close()

	log(
		"downloading youtu.be/%s video size:%dmb quality:%s bitrate:%dkbps duration:%s language:%#v",
		v.Id,
		ytstreamsize>>20,
		videoFormat.QualityLabel,
		videoFormat.Bitrate>>10,
		vinfo.Duration,
		videoFormat.LanguageDisplayName(),
	)

	var tgvideo *TgVideo
	tgvideoCaption := fmt.Sprintf("%s %s"+NL+"youtu.be/%s %s %s", vinfo.Title, vinfo.PublishDate.Format("2006/01/02"), v.Id, vinfo.Duration, videoFormat.QualityLabel)
	if v.PlaylistId != "" && v.PlaylistTitle != "" {
		tgvideoCaption = fmt.Sprintf("%d/%d %s "+NL, v.PlaylistIndex+1, v.PlaylistSize, v.PlaylistTitle) + tgvideoCaption
	}

	tgvideoFilename := fmt.Sprintf("%s.%s.mp4", ts(), v.Id)
	tgvideoFile, err := os.OpenFile(tgvideoFilename, os.O_RDWR|os.O_CREATE, 0600)
	if err != nil {
		return fmt.Errorf("os.OpenFile: %w", err)
	}

	if DEBUG {
		downloadingmessagetext := fmt.Sprintf("%s"+NL+"youtu.be/%s %s %s"+NL+"downloading", vinfo.Title, v.Id, vinfo.Duration, videoFormat.QualityLabel)
		if v.PlaylistId != "" && v.PlaylistTitle != "" {
			downloadingmessagetext = fmt.Sprintf("%d/%d %s "+NL, v.PlaylistIndex+1, v.PlaylistSize, v.PlaylistTitle) + downloadingmessagetext
		}
		if downloadingmessage, err := tgsendMessage(downloadingmessagetext, m.Chat.Id, "", 0); err == nil && downloadingmessage != nil {
			tgdeleteMessages = append(tgdeleteMessages, TgChatMessageId{m.Chat.Id, downloadingmessage.MessageId})
		}
	}

	t0 := time.Now()
	_, err = io.Copy(tgvideoFile, ytstream)
	if err != nil {
		return fmt.Errorf("download youtu.be/%s video: %w", v.Id, err)
	}

	if err := ytstream.Close(); err != nil {
		log("ytstream.Close: %v", err)
	}
	if err := tgvideoFile.Close(); err != nil {
		return fmt.Errorf("os.File.Close: %w", err)
	}

	log("downloaded youtu.be/%s video in %v", v.Id, time.Since(t0).Truncate(time.Second))
	if DEBUG {
		downloadedmessagetext := fmt.Sprintf("%s"+NL+"youtu.be/%s %s %s"+NL+"downloaded video in %v", vinfo.Title, v.Id, vinfo.Duration, videoFormat.QualityLabel, time.Since(t0).Truncate(time.Second))
		if targetVideoBitrateKbps > 0 {
			downloadedmessagetext += NL + fmt.Sprintf("transcoding to audio:%dkbps video:%dkbps", TgAudioBitrateKbps, targetVideoBitrateKbps)
		}
		downloadedmessage, err := tgsendMessage(downloadedmessagetext, m.Chat.Id, "", 0)
		if err == nil && downloadedmessage != nil {
			tgdeleteMessages = append(tgdeleteMessages, TgChatMessageId{m.Chat.Id, downloadedmessage.MessageId})
		}
	}

	if FfmpegPath != "" && targetVideoBitrateKbps > 0 {
		filename2 := fmt.Sprintf("%s.%s.v%dk.a%dk.mp4", ts(), v.Id, targetVideoBitrateKbps, TgAudioBitrateKbps)
		err := FfmpegTranscode(tgvideoFilename, filename2, targetVideoBitrateKbps, TgAudioBitrateKbps)
		if err != nil {
			return fmt.Errorf("FfmpegTranscode `%s`: %w", tgvideoFilename, err)
		}
		tgvideoCaption += NL + fmt.Sprintf("(transcoded to video:%dkbps audio:%dkbps)", targetVideoBitrateKbps, TgAudioBitrateKbps)
		if err := os.Remove(tgvideoFilename); err != nil {
			log("os.Remove `%s`: %v", tgvideoFilename, err)
		}
		tgvideoFilename = filename2
	}

	tgvideoReader, err := os.Open(tgvideoFilename)
	if err != nil {
		return fmt.Errorf("os.Open: %w", err)
	}
	defer tgvideoReader.Close()

	tgvideo, err = tgsendVideoFile(
		m.Chat.Id,
		tgvideoCaption,
		tgvideoReader,
		videoFormat.Width,
		videoFormat.Height,
		vinfo.Duration,
	)
	if err != nil {
		return fmt.Errorf("tgsendVideoFile: %w", err)
	}

	if err := tgvideoReader.Close(); err != nil {
		log("os.File.Close: %v", err)
	}
	if err := os.Remove(tgvideoFilename); err != nil {
		log("os.Remove: %v", err)
	}

	if tgvideo.FileId == "" {
		return fmt.Errorf("tgsendVideoFile: file_id empty")
	}

	return nil
}

func postAudio(v YtVideo, vinfo *ytdl.Video, m TgMessage) error {
	var audioFormat, audioSmallestFormat ytdl.Format

	var tgdeleteMessages []TgChatMessageId
	defer func(mm *[]TgChatMessageId) {
		for _, cm := range *mm {
			tgdeleteMessage(cm.ChatId, cm.MessageId)
		}
	}(&tgdeleteMessages)

	for _, f := range vinfo.Formats.WithAudioChannels() {
		if !strings.Contains(f.MimeType, "/mp4") {
			continue
		}
		fsize := f.ContentLength
		if fsize == 0 {
			fsize = int64(f.Bitrate / 8 * int(vinfo.Duration.Seconds()))
		}
		if !strings.HasPrefix(f.MimeType, "audio/mp4") {
			continue
		}
		flang := strings.ToLower(f.LanguageDisplayName())
		log("format: ContentLength:%dmb Language:%#v", f.ContentLength>>20, flang)
		if flang != "" {
			skip := true
			for _, l := range DownloadLanguages {
				if strings.Contains(flang, l) {
					skip = false
				}
			}
			if skip {
				continue
			}
		}
		if audioSmallestFormat.ItagNo == 0 || f.Bitrate < audioSmallestFormat.Bitrate {
			audioSmallestFormat = f
		}
		if fsize < TgMaxFileSizeBytes && f.Bitrate > audioFormat.Bitrate {
			audioFormat = f
		}
	}

	var targetAudioBitrateKbps int64
	if audioFormat.ItagNo == 0 {
		audioFormat = audioSmallestFormat
		targetAudioBitrateKbps = int64(((TgMaxFileSizeBytes * 8) / int64(vinfo.Duration.Seconds()+1)) / 1024)
	}

	ytstream, ytstreamsize, err := YtCl.GetStreamContext(Ctx, vinfo, &audioFormat)
	if err != nil {
		return fmt.Errorf("GetStreamContext: %w", err)
	}
	defer ytstream.Close()

	if ytstreamsize == 0 {
		return fmt.Errorf("GetStreamContext: stream size is zero")
	}

	log(
		"downloading youtu.be/%s audio size:%dmb bitrate:%dkbps duration:%s language:%#v",
		v.Id,
		ytstreamsize>>20,
		audioFormat.Bitrate>>10,
		vinfo.Duration,
		audioFormat.LanguageDisplayName(),
	)

	var tgaudio *TgAudio
	tgaudioCaption := fmt.Sprintf("%s %s"+NL+"youtu.be/%s %s %dkbps", vinfo.Title, vinfo.PublishDate.Format("2006/01/02"), v.Id, vinfo.Duration, audioFormat.Bitrate/1024)
	if v.PlaylistId != "" && v.PlaylistTitle != "" {
		tgaudioCaption = fmt.Sprintf("%d/%d %s "+NL, v.PlaylistIndex+1, v.PlaylistSize, v.PlaylistTitle) + tgaudioCaption
	}

	tgaudioFilename := fmt.Sprintf("%s.%s.m4a", ts(), v.Id)
	tgaudioFile, err := os.OpenFile(tgaudioFilename, os.O_RDWR|os.O_CREATE, 0600)
	if err != nil {
		return fmt.Errorf("create file: %w", err)
	}

	if DEBUG {
		downloadingmessagetext := fmt.Sprintf("%s"+NL+"youtu.be/%s %s %dkbps"+NL+"downloading", vinfo.Title, v.Id, vinfo.Duration, audioFormat.Bitrate/1024)
		if v.PlaylistId != "" && v.PlaylistTitle != "" {
			downloadingmessagetext = fmt.Sprintf("%d/%d %s "+NL, v.PlaylistIndex+1, v.PlaylistSize, v.PlaylistTitle) + downloadingmessagetext
		}
		if downloadingmessage, err := tgsendMessage(downloadingmessagetext, m.Chat.Id, "", 0); err == nil && downloadingmessage != nil {
			tgdeleteMessages = append(tgdeleteMessages, TgChatMessageId{m.Chat.Id, downloadingmessage.MessageId})
		}
	}

	t0 := time.Now()
	_, err = io.Copy(tgaudioFile, ytstream)
	if err != nil {
		return fmt.Errorf("download youtu.be/%s audio: %w", v.Id, err)
	}

	if err := ytstream.Close(); err != nil {
		log("ytstream.Close: %v", err)
	}
	if err := tgaudioFile.Close(); err != nil {
		return fmt.Errorf("os.File.Close: %w", err)
	}

	log("downloaded youtu.be/%s audio in %v", v.Id, time.Since(t0).Truncate(time.Second))
	if DEBUG {
		downloadedmessagetext := fmt.Sprintf("%s"+NL+"youtu.be/%s %s %dkbps"+NL+"downloaded audio in %s", vinfo.Title, v.Id, vinfo.Duration, audioFormat.Bitrate/1024, time.Since(t0).Truncate(time.Second))
		if targetAudioBitrateKbps > 0 {
			downloadedmessagetext += NL + fmt.Sprintf("transcoding to audio:%dkbps", targetAudioBitrateKbps)
		}
		downloadedmessage, err := tgsendMessage(downloadedmessagetext, m.Chat.Id, "", 0)
		if err == nil && downloadedmessage != nil {
			tgdeleteMessages = append(tgdeleteMessages, TgChatMessageId{m.Chat.Id, downloadedmessage.MessageId})
		}
	}

	if FfmpegPath != "" && targetAudioBitrateKbps > 0 {
		filename2 := fmt.Sprintf("%s.%s.a%dk.m4a", ts(), v.Id, targetAudioBitrateKbps)
		err := FfmpegTranscode(tgaudioFilename, filename2, 0, targetAudioBitrateKbps)
		if err != nil {
			return fmt.Errorf("FfmpegTranscode `%s`: %w", tgaudioFilename, err)
		}
		tgaudioCaption += NL + fmt.Sprintf("(transcoded to audio:%dkbps)", targetAudioBitrateKbps)
		if err := os.Remove(tgaudioFilename); err != nil {
			log("os.Remove `%s`: %v", tgaudioFilename, err)
		}
		tgaudioFilename = filename2
	}

	tgaudioReader, err := os.Open(tgaudioFilename)
	if err != nil {
		return fmt.Errorf("os.Open: %w", err)
	}
	defer tgaudioReader.Close()

	tgaudio, err = tgsendAudioFile(
		m.Chat.Id,
		tgaudioCaption,
		tgaudioReader,
		vinfo.Author,
		vinfo.Title,
		vinfo.Duration,
	)
	if err != nil {
		return fmt.Errorf("tgsendAudioFile: %w", err)
	}

	if err := tgaudioReader.Close(); err != nil {
		log("os.File.Close: %v", err)
	}
	if err := os.Remove(tgaudioFilename); err != nil {
		log("os.Remove: %v", err)
	}

	if tgaudio == nil {
		return fmt.Errorf("tgsendAudioFile: result is nil")
	}
	if tgaudio.FileId == "" {
		return fmt.Errorf("tgsendAudioFile: file_id empty")
	}

	return nil
}

func getList(ytlistid string) (ytitems []YtVideo, err error) {
	// https://developers.google.com/youtube/v3/docs/playlists
	var PlaylistUrl = fmt.Sprintf("https://www.googleapis.com/youtube/v3/playlists?maxResults=%d&part=snippet&id=%s&key=%s", YtMaxResults, ytlistid, YtKey)
	var playlists YtPlaylists
	err = getJson(PlaylistUrl, &playlists, nil)
	if err != nil {
		return nil, err
	}

	if len(playlists.Items) < 1 {
		return nil, fmt.Errorf("no playlists found with provided id %s", ytlistid)
	}
	if len(playlists.Items) > 1 {
		return nil, fmt.Errorf("more than one (%d) playlists found with provided id %s", len(playlists.Items), ytlistid)
	}

	log("playlist title: %s", playlists.Items[0].Snippet.Title)

	listtitle := playlists.Items[0].Snippet.Title

	var videos []YtPlaylistItemSnippet
	nextPageToken := ""

	for nextPageToken != "" || len(videos) == 0 {
		// https://developers.google.com/youtube/v3/docs/playlistItems
		var PlaylistItemsUrl = fmt.Sprintf("https://www.googleapis.com/youtube/v3/playlistItems?maxResults=%d&part=snippet&playlistId=%s&key=%s&pageToken=%s", YtMaxResults, ytlistid, YtKey, nextPageToken)

		var playlistItems YtPlaylistItems
		err = getJson(PlaylistItemsUrl, &playlistItems, nil)
		if err != nil {
			return nil, err
		}

		if playlistItems.NextPageToken != nextPageToken {
			nextPageToken = playlistItems.NextPageToken
		} else {
			nextPageToken = ""
		}

		for _, i := range playlistItems.Items {
			videos = append(videos, i.Snippet)
		}
	}

	//sort.Slice(videos, func(i, j int) bool { return videos[i].PublishedAt < videos[j].PublishedAt })

	for _, vid := range videos {
		ytitems = append(
			ytitems,
			YtVideo{
				Id:            vid.ResourceId.VideoId,
				PlaylistId:    vid.PlaylistId,
				PlaylistIndex: vid.Position,
				PlaylistSize:  int64(len(videos)),
				PlaylistTitle: listtitle,
			},
		)
	}

	return ytitems, nil
}

func safestring(s string) (t string) {
	for _, r := range s {
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) {
			r = '.'
		}
		t = t + string(r)
	}

	if len([]rune(t)) > 40 {
		t = string([]rune(t)[:40])
	}

	return t
}

func tgsendVideoFile(chatid int64, caption string, video io.Reader, width, height int, duration time.Duration) (tgvideo *TgVideo, err error) {
	piper, pipew := io.Pipe()
	mpartw := multipart.NewWriter(pipew)

	var mparterr error
	go func(err error) {
		defer func() {
			if mparterr != nil {
				log("mparterr: %v", err)
			}
		}()

		var formw io.Writer

		defer pipew.Close()

		// chat_id
		formw, err = mpartw.CreateFormField("chat_id")
		if err != nil {
			err = fmt.Errorf("CreateFormField(`chat_id`): %w", err)
			return
		}
		_, err = formw.Write([]byte(strconv.Itoa(int(chatid))))
		if err != nil {
			err = fmt.Errorf("Write(chat_id): %w", err)
			return
		}

		// caption
		formw, err = mpartw.CreateFormField("caption")
		if err != nil {
			err = fmt.Errorf("CreateFormField(`caption`): %w", err)
			return
		}
		_, err = formw.Write([]byte(caption))
		if err != nil {
			err = fmt.Errorf("Write(caption): %w", err)
			return
		}

		// width
		formw, err = mpartw.CreateFormField("width")
		if err != nil {
			err = fmt.Errorf("CreateFormField(`width`): %w", err)
			return
		}
		_, err = formw.Write([]byte(strconv.Itoa(width)))
		if err != nil {
			err = fmt.Errorf("Write(width): %w", err)
			return
		}

		// height
		formw, err = mpartw.CreateFormField("height")
		if err != nil {
			err = fmt.Errorf("CreateFormField(`height`): %w", err)
			return
		}
		_, err = formw.Write([]byte(strconv.Itoa(height)))
		if err != nil {
			err = fmt.Errorf("Write(height): %w", err)
			return
		}

		// video
		formw, err = mpartw.CreateFormFile("video", safestring(caption))
		if err != nil {
			err = fmt.Errorf("CreateFormFile('video'): %w", err)
			return
		}
		_, err = io.Copy(formw, video)
		if err != nil {
			err = fmt.Errorf("Copy video: %w", err)
			return
		}

		// duration
		formw, err = mpartw.CreateFormField("duration")
		if err != nil {
			err = fmt.Errorf("CreateFormField(`duration`): %w", err)
			return
		}
		_, err = formw.Write([]byte(strconv.Itoa(int(duration.Seconds()))))
		if err != nil {
			err = fmt.Errorf("Write(duration): %w", err)
			return
		}

		if err := mpartw.Close(); err != nil {
			err = fmt.Errorf("multipart.Writer.Close: %w", err)
			return
		}
	}(mparterr)

	t0 := time.Now()

	resp, err := HttpClient.Post(
		fmt.Sprintf("%s/bot%s/sendVideo", TelegramApiUrlBase, TgToken),
		mpartw.FormDataContentType(),
		piper,
	)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if mparterr != nil {
		return nil, err
	}

	var tgresp TgResponse
	err = json.NewDecoder(resp.Body).Decode(&tgresp)
	if err != nil {
		return nil, fmt.Errorf("Decode: %w", err)
	}
	if !tgresp.Ok {
		return nil, fmt.Errorf("sendVideo: %s", tgresp.Description)
	}

	msg := tgresp.Result
	tgvideo = &msg.Video
	if tgvideo.FileId == "" {
		return nil, fmt.Errorf("sendVideo: Video.FileId empty")
	}

	log("sent the video to telegram in %v", time.Since(t0).Truncate(time.Second))

	return tgvideo, nil
}

func tgsendAudioFile(chatid int64, caption string, audio io.Reader, performer, title string, duration time.Duration) (tgaudio *TgAudio, err error) {
	piper, pipew := io.Pipe()
	mpartw := multipart.NewWriter(pipew)

	var mparterr error
	go func(err error) {
		defer func() {
			if mparterr != nil {
				log("mparterr: %v", err)
			}
		}()

		var formw io.Writer

		defer pipew.Close()

		// chat_id
		formw, err = mpartw.CreateFormField("chat_id")
		if err != nil {
			err = fmt.Errorf("CreateFormField(`chat_id`): %w", err)
			return
		}
		_, err = formw.Write([]byte(strconv.Itoa(int(chatid))))
		if err != nil {
			err = fmt.Errorf("Write(chat_id): %w", err)
			return
		}

		// performer
		formw, err = mpartw.CreateFormField("performer")
		if err != nil {
			err = fmt.Errorf("CreateFormField(`performer`): %w", err)
			return
		}
		_, err = formw.Write([]byte(performer))
		if err != nil {
			err = fmt.Errorf("Write(performer): %w", err)
			return
		}

		// title
		formw, err = mpartw.CreateFormField("title")
		if err != nil {
			err = fmt.Errorf("CreateFormField(`title`): %w", err)
			return
		}
		_, err = formw.Write([]byte(title))
		if err != nil {
			err = fmt.Errorf("Write(title): %w", err)
			return
		}

		// caption
		formw, err = mpartw.CreateFormField("caption")
		if err != nil {
			err = fmt.Errorf("CreateFormField(`caption`): %w", err)
			return
		}
		_, err = formw.Write([]byte(caption))
		if err != nil {
			err = fmt.Errorf("Write(caption): %w", err)
			return
		}

		// audio
		formw, err = mpartw.CreateFormFile("audio", safestring(fmt.Sprintf("%s.%s", performer, title)))
		if err != nil {
			err = fmt.Errorf("CreateFormFile('audio'): %w", err)
			return
		}
		_, err = io.Copy(formw, audio)
		if err != nil {
			err = fmt.Errorf("Copy audio: %w", err)
			return
		}

		// duration
		formw, err = mpartw.CreateFormField("duration")
		if err != nil {
			err = fmt.Errorf("CreateFormField(`duration`): %w", err)
			return
		}
		_, err = formw.Write([]byte(strconv.Itoa(int(duration.Seconds()))))
		if err != nil {
			err = fmt.Errorf("Write(duration): %w", err)
			return
		}

		if err := mpartw.Close(); err != nil {
			err = fmt.Errorf("multipart.Writer.Close: %w", err)
			return
		}
	}(mparterr)

	t0 := time.Now()

	resp, err := HttpClient.Post(
		fmt.Sprintf("%s/bot%s/sendAudio", TelegramApiUrlBase, TgToken),
		mpartw.FormDataContentType(),
		piper,
	)
	if err != nil {
		if regexp.MustCompile("Too Many Requests: retry after [0-9]+$").MatchString(fmt.Sprintf("%s", err)) {
			log("WARNING telegram api too many requests: sleeping 33 seconds")
			time.Sleep(33 * time.Second)
		}
		return nil, err
	}
	defer resp.Body.Close()

	if mparterr != nil {
		return nil, err
	}

	var tgresp TgResponse
	err = json.NewDecoder(resp.Body).Decode(&tgresp)
	if err != nil {
		return nil, fmt.Errorf("Decode: %w", err)
	}
	if !tgresp.Ok {
		return nil, fmt.Errorf("sendAudio: %s", tgresp.Description)
	}

	msg := tgresp.Result
	tgaudio = &msg.Audio
	if tgaudio.FileId == "" {
		return nil, fmt.Errorf("sendAudio: Audio.FileId empty")
	}

	log("sent the audio to telegram in %v", time.Since(t0).Truncate(time.Second))

	return tgaudio, nil
}

func tgsendMessage(text string, chatid int64, parsemode string, replytomessageid int64) (msg *TgMessage, err error) {
	// https://core.telegram.org/bots/api/#sendmessage
	// https://core.telegram.org/bots/api/#formatting-options
	sendMessage := map[string]interface{}{
		"chat_id":                  chatid,
		"text":                     text,
		"parse_mode":               parsemode,
		"disable_web_page_preview": true,
	}
	if replytomessageid != 0 {
		sendMessage["reply_to_message_id"] = replytomessageid
	}
	sendMessageJSON, err := json.Marshal(sendMessage)
	if err != nil {
		return nil, err
	}

	var tgresp TgResponse
	err = postJson(
		fmt.Sprintf("%s/bot%s/sendMessage", TelegramApiUrlBase, TgToken),
		bytes.NewBuffer(sendMessageJSON),
		&tgresp,
	)
	if err != nil {
		return nil, err
	}

	if !tgresp.Ok {
		return nil, fmt.Errorf("sendMessage: %s", tgresp.Description)
	}

	msg = tgresp.Result

	return msg, nil
}

func tgdeleteMessage(chatid, messageid int64) error {
	deleteMessage := map[string]interface{}{
		"chat_id":    chatid,
		"message_id": messageid,
	}
	deleteMessageJSON, err := json.Marshal(deleteMessage)
	if err != nil {
		return err
	}

	var tgresp TgResponseShort
	err = postJson(
		fmt.Sprintf("%s/bot%s/deleteMessage", TelegramApiUrlBase, TgToken),
		bytes.NewBuffer(deleteMessageJSON),
		&tgresp,
	)
	if err != nil {
		return fmt.Errorf("postJson: %w", err)
	}

	if !tgresp.Ok {
		return fmt.Errorf("deleteMessage: %s", tgresp.Description)
	}

	return nil
}

func FfmpegTranscode(filename, filename2 string, videoBitrateKbps, audioBitrateKbps int64) (err error) {
	if videoBitrateKbps > 0 {
		log("transcoding to video:%dkbps audio:%dkbps ", videoBitrateKbps, audioBitrateKbps)
	} else if audioBitrateKbps > 0 {
		log("transcoding to audio:%dkbps", audioBitrateKbps)
	} else {
		return fmt.Errorf("empty both videoBitrateKbps and audioBitrateKbps")
	}

	ffmpegArgs := append(FfmpegGlobalOptions,
		"-i", filename,
		"-f", "mp4",
	)
	if videoBitrateKbps > 0 {
		ffmpegArgs = append(ffmpegArgs,
			"-c:v", "h264",
			"-b:v", fmt.Sprintf("%dk", videoBitrateKbps),
		)
	}
	if audioBitrateKbps > 0 {
		ffmpegArgs = append(ffmpegArgs,
			"-c:a", "aac",
			"-b:a", fmt.Sprintf("%dk", audioBitrateKbps),
		)
	}
	ffmpegArgs = append(ffmpegArgs,
		filename2,
	)

	ffmpegCmd := exec.Command(FfmpegPath, ffmpegArgs...)

	ffmpegCmdStderrPipe, err := ffmpegCmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("ffmpeg StderrPipe: %w", err)
	}

	t0 := time.Now()
	err = ffmpegCmd.Start()
	if err != nil {
		return fmt.Errorf("ffmpeg Start: %w", err)
	}

	log("started command `%s`", ffmpegCmd.String())

	_, err = io.Copy(os.Stderr, ffmpegCmdStderrPipe)
	if err != nil {
		log("copy from ffmpeg stderr: %w", err)
	}

	err = ffmpegCmd.Wait()
	if err != nil {
		return fmt.Errorf("ffmpeg Wait: %w", err)
	}

	log("transcoded in %v", time.Since(t0).Truncate(time.Second))

	return nil
}
