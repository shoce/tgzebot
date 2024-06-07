/*

https://pkg.go.dev/github.com/kkdai/youtube/v2/
https://core.telegram.org/bots/api/

https://johnvansickle.com/ffmpeg/

curl -s -S -L https://johnvansickle.com/ffmpeg/releases/ffmpeg-release-amd64-static.tar.xz | tar -x -J
mv ./ffmpeg-*-amd64-static/ffmpeg ./ffmpeg

go mod init github.com/shoce/tgzebot
go get -a -u -v
go get github.com/kkdai/youtube/v2@master
go mod tidy

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
	yaml "gopkg.in/yaml.v3"
)

const (
	NL = "\n"
)

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

type TgUpdate struct {
	UpdateId          int64     `json:"update_id"`
	Message           TgMessage `json:"message"`
	EditedMessage     TgMessage `json:"edited_message"`
	ChannelPost       TgMessage `json:"channel_post"`
	EditedChannelPost TgMessage `json:"edited_channel_post"`
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

var (
	DEBUG bool

	YamlConfigPath = "tgzebot.yaml"

	KvToken       string
	KvAccountId   string
	KvNamespaceId string

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

	TgMaxFileSizeBytes int64    = 49 << 20
	TgAudioBitrateKbps int64    = 50
	DownloadLanguages  []string = []string{"english", "german", "russian", "ukrainian"}

	FfmpegPath          string = "./ffmpeg"
	FfmpegGlobalOptions        = []string{"-v", "error"}

	TgCommandChannels             string
	TgCommandChannelsPromoteAdmin string

	TgQuest1    string
	TgQuest1Key string
	TgQuest2    string
	TgQuest2Key string
	TgQuest3    string
	TgQuest3Key string

	TgAllChannelsChatIds []int64

	TzBiel *time.Location
)

func beats(td time.Duration) int {
	const beat = time.Duration(24) * time.Hour / 1000
	return int(td / beat)
}

func ts() string {
	t := time.Now().In(TzBiel)
	ts := fmt.Sprintf(
		"%03dy"+"%02dm"+"%02dd"+"%db",
		t.Year()%1000, t.Month(), t.Day(),
		beats(time.Since(time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, TzBiel))),
	)
	return ts
}

func tsversion() string {
	t := time.Now().In(TzBiel)
	v := fmt.Sprintf(
		"%03d.%02d%02d.%d",
		t.Year()%1000, t.Month(), t.Day(),
		beats(time.Since(time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, TzBiel))),
	)
	return v
}

func log(msg interface{}, args ...interface{}) {
	t := time.Now().Local()
	ts := fmt.Sprintf(
		"%03d."+"%02d%02d."+"%02d"+"%02d.",
		t.Year()%1000, t.Month(), t.Day(), t.Hour(), t.Minute(),
	)
	msgtext := fmt.Sprintf("%s %s", ts, msg) + NL
	fmt.Fprintf(os.Stderr, msgtext, args...)
}

func getJson(url string, target interface{}, respjson *string) (err error) {
	resp, err := HttpClient.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	respBody := bytes.NewBuffer(nil)
	_, err = io.Copy(respBody, resp.Body)
	if err != nil {
		return fmt.Errorf("io.Copy: %w", err)
	}

	err = json.NewDecoder(respBody).Decode(target)
	if err != nil {
		return fmt.Errorf("json.Decoder.Decode: %w", err)
	}

	log("getJson %s response ContentLength:%d Body:"+NL+"%s", url, resp.ContentLength, respBody.String())
	if respjson != nil {
		*respjson = respBody.String()
		log("getJson %s respjson:"+NL+"%s", url, *respjson)
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

	respBody := bytes.NewBuffer(nil)
	_, err = io.Copy(respBody, resp.Body)
	if err != nil {
		return fmt.Errorf("io.Copy: %w", err)
	}

	err = json.NewDecoder(respBody).Decode(target)
	if err != nil {
		return fmt.Errorf("Decode: %w", err)
	}

	return nil
}

func GetVar(name string) (value string) {
	if DEBUG {
		log("DEBUG GetVar: %s", name)
	}

	var err error

	value = os.Getenv(name)
	if value != "" {
		return value
	}

	if YamlConfigPath != "" {
		value, err = YamlGet(name)
		if err != nil {
			log("WARNING GetVar YamlGet %s: %v", name, err)
		}
		if value != "" {
			return value
		}
	}

	if KvToken != "" && KvAccountId != "" && KvNamespaceId != "" {
		if v, err := KvGet(name); err != nil {
			log("WARNING GetVar KvGet %s: %v", name, err)
			return ""
		} else {
			value = v
		}
	}

	return value
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
		//log("WARNING: os.Open config file %s: %v", YamlConfigPath, err)
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	defer configf.Close()

	configm := make(map[interface{}]interface{})
	if err = yaml.NewDecoder(configf).Decode(&configm); err != nil {
		//log("WARNING: yaml.Decode %s: %v", YamlConfigPath, err)
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
			log("WARNING: yaml.Decode %s: %v", YamlConfigPath, err)
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
				log("WARNING: yaml.Encoder.Encode: %v", err)
				return err
			}
		} else {
			log("WARNING: os.Create config file %s: %v", YamlConfigPath, err)
			return err
		}
	} else {
		log("WARNING: os.Open config file %s: %v", YamlConfigPath, err)
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

func init() {
	var err error

	TzBiel = time.FixedZone("Biel", 60*60)

	if os.Getenv("YamlConfigPath") != "" {
		YamlConfigPath = os.Getenv("YamlConfigPath")
	}
	if YamlConfigPath == "" {
		log("WARNING: YamlConfigPath empty")
	}

	KvToken = GetVar("KvToken")
	if KvToken == "" {
		log("WARNING: KvToken empty")
	}

	KvAccountId = GetVar("KvAccountId")
	if KvAccountId == "" {
		log("WARNING: KvAccountId empty")
	}

	KvNamespaceId = GetVar("KvNamespaceId")
	if KvNamespaceId == "" {
		log("WARNING: KvNamespaceId empty")
	}

	Ctx = context.TODO()

	YtProxy := http.ProxyFromEnvironment
	if GetVar("YtProxyList") != "" {
		pp := strings.Split(GetVar("YtProxyList"), " ")
		rand.Seed(time.Now().UnixNano())
		if err := SetVar("YtProxy", pp[rand.Intn(len(pp))]); err != nil {
			log("WARNING: SetVar YtProxy: %v", err)
		}
	}
	YtProxyUrl := GetVar("YtProxy")
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

	ytRe = regexp.MustCompile(YtReString)
	ytlistRe = regexp.MustCompile(YtListReString)

	TgToken = GetVar("TgToken")
	if TgToken == "" {
		log("ERROR: TgToken empty")
		os.Exit(1)
	}

	for _, s := range strings.Split(GetVar("TgUpdateLog"), " ") {
		if s == "" {
			continue
		}
		i, err := strconv.ParseInt(s, 10, 0)
		if err != nil {
			log("WARNING: %v", err)
			continue
		}
		TgUpdateLog = append(TgUpdateLog, i)
	}

	if GetVar("TgZeChatId") == "" {
		log("ERROR: TgZeChatId empty")
		os.Exit(1)
	} else {
		TgZeChatId, err = strconv.ParseInt(GetVar("TgZeChatId"), 10, 0)
		if err != nil {
			log("ERROR: invalid TgZeChatId: %v", err)
			os.Exit(1)
		}
	}

	TgCommandChannels = GetVar("TgCommandChannels")
	if TgCommandChannels == "" {
		log("ERROR: TgCommandChannels empty")
		os.Exit(1)
	}

	TgCommandChannelsPromoteAdmin = GetVar("TgCommandChannelsPromoteAdmin")
	if TgCommandChannelsPromoteAdmin == "" {
		log("ERROR: TgCommandChannelsPromoteAdmin empty")
		os.Exit(1)
	}

	TgQuest1 = GetVar("TgQuest1")
	TgQuest1Key = GetVar("TgQuest1Key")

	TgQuest2 = GetVar("TgQuest2")
	TgQuest2Key = GetVar("TgQuest2Key")

	TgQuest3 = GetVar("TgQuest3")
	TgQuest3Key = GetVar("TgQuest3Key")

	YtKey = GetVar("YtKey")
	if YtKey == "" {
		log("ERROR: YtKey empty")
		os.Exit(1)
	}

	if GetVar("YtMaxResults") != "" {
		YtMaxResults, err = strconv.ParseInt(GetVar("YtMaxResults"), 10, 0)
		if err != nil {
			log("ERROR: invalid YtMaxResults: %v", err)
			os.Exit(1)
		}
	}

	if GetVar("YtHttpClientUserAgent") != "" {
		YtHttpClientUserAgent = GetVar("YtHttpClientUserAgent")
	}
	if GetVar("YtReString") != "" {
		YtReString = GetVar("YtReString")
	}
	if GetVar("YtListReString") != "" {
		YtListReString = GetVar("YtListReString")
	}

	if GetVar("FfmpegPath") != "" {
		FfmpegPath = GetVar("FfmpegPath")
	}
	if GetVar("FfmpegGlobalOptions") != "" {
		FfmpegGlobalOptions = strings.Split(GetVar("FfmpegGlobalOptions"), " ")
	}

	for _, s := range strings.Split(GetVar("TgAllChannelsChatIds"), " ") {
		if i, err := strconv.ParseInt(s, 10, 0); err != nil {
			log("WARNING: invalid TgAllChannelsChatIds: %v", err)
			continue
		} else {
			TgAllChannelsChatIds = append(TgAllChannelsChatIds, i)
		}
	}
}

func tggetUpdates() (uu []TgUpdate, tgrespjson string, err error) {
	var offset int64
	if len(TgUpdateLog) > 0 {
		offset = TgUpdateLog[len(TgUpdateLog)-1] + 1
	}
	getUpdatesUrl := fmt.Sprintf("https://api.telegram.org/bot%s/getUpdates?offset=%d", TgToken, offset)
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
	getChatUrl := fmt.Sprintf("https://api.telegram.org/bot%s/getChat?chat_id=%d", TgToken, chatid)
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
		fmt.Sprintf("https://api.telegram.org/bot%s/promoteChatMember", TgToken),
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
	getChatAdministratorsUrl := fmt.Sprintf("https://api.telegram.org/bot%s/getChatAdministrators?chat_id=%d", TgToken, chatid)
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

func main() {
	var err error

	sigterm := make(chan os.Signal, 1)
	signal.Notify(sigterm, syscall.SIGTERM)
	go func(sigterm chan os.Signal) {
		<-sigterm
		tgsendMessage(fmt.Sprintf("%s: sigterm received", os.Args[0]), TgZeChatId, "", 0)
		log("sigterm received")
		os.Exit(1)
	}(sigterm)

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

		if len(TgUpdateLog) > 0 && TgUpdateLog[len(TgUpdateLog)-1] > u.UpdateId {
			log("This Update was processed already, skipping")
			continue
		}

		if len(TgUpdateLog) > 1 && TgUpdateLog[len(TgUpdateLog)-1] == u.UpdateId && TgUpdateLog[len(TgUpdateLog)-2] == u.UpdateId {
			//log("TgUpdateLog: %v", TgUpdateLog)
			log("This Update was tried twice already, skipping")
			continue
		}

		TgUpdateLog = append(TgUpdateLog, u.UpdateId)
		if len(TgUpdateLog) > 4 {
			TgUpdateLog = TgUpdateLog[len(TgUpdateLog)-4:]
		}
		ltss := []string{}
		for _, i := range TgUpdateLog {
			ltss = append(ltss, fmt.Sprintf("%d", i))
		}
		if err := SetVar("TgUpdateLog", strings.Join(ltss, " ")); err != nil {
			log("WARNING: SetVar TgUpdateLog: %v", err)
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
		} else {
			log("Unsupported type of update received:"+NL+"%s", respjson)
			_, err = tgsendMessage(fmt.Sprintf("Unsupported type of update received:"+NL+"```"+NL+"%s"+NL+"```", respjson), TgZeChatId, "", 0)
			if err != nil {
				log("tgsendMessage: %v", err)
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

			if GetVar("TgAllChannelsChatIds") != s {
				if err := SetVar("TgAllChannelsChatIds", s); err != nil {
					log("WARNING: SetVar TgAllChannelsChatIds: %v", err)
				}
			}
		}

		log("Message text: `%s`", m.Text)

		shouldreport := true
		if m.From.Id == TgZeChatId {
			shouldreport = false
		}
		var chatadmins string
		if aa, err := tggetChatAdministrators(m.Chat.Id); err == nil {
			for _, a := range aa {
				chatadmins += fmt.Sprintf("@%s;id:%d;status:%s ", a.User.Username, a.User.Id, a.Status)
				if a.User.Id == TgZeChatId {
					shouldreport = false
				}
			}
		} else {
			log("tggetChatAdministrators: %v", err)
		}
		if shouldreport {
			report := fmt.Sprintf(
				"%s"+NL+NL+
					"%s"+NL+
					"from: @%s id:%d"+NL+
					"chat: id:%d type:%s title:%s"+NL+
					"chat admins: %s",
				m.Text,
				func(em bool) (s string) {
					if em {
						return "edited message"
					}
					return "new message"
				}(iseditmessage),
				m.From.Username, m.From.Id,
				m.Chat.Id, m.Chat.Type, m.Chat.Title,
				chatadmins,
			)
			_, err = tgsendMessage(report, TgZeChatId, "", 0)
			if err != nil {
				log("tgsendMessage: %v", err)
				continue
			}
		}

		if strings.TrimSpace(m.Text) == "/id" {
			_, err = tgsendMessage(
				fmt.Sprintf("username `%s`"+NL+"user id `%d`"+NL+"chat id `%d`", m.From.Username, m.From.Id, m.Chat.Id),
				m.Chat.Id, "MarkdownV2", 0,
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
			_, err = tgsendMessage(fmt.Sprintf("Ok for %d of total %d channels.", totalok, total), m.Chat.Id, "", m.MessageId)
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
					log("GetVideoContext: %v", err)
					postingerr = err
					break
				}

				if downloadvideo {
					err = postVideo(v, vinfo, m)
					if err != nil {
						log("postVideo: %v", err)
						postingerr = err
						break
					}
				} else {
					err = postAudio(v, vinfo, m)
					if err != nil {
						log("postAudio: %v", err)
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
				_, err = tgsendMessage(fmt.Sprintf("%s\nError: %v", m.Text, postingerr), m.Chat.Id, "", 0)
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
		log("format: ContentLength:%dMB Language:%#v", f.ContentLength>>20, flang)
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
		"Downloading youtube video size:%dMB quality:%s bitrate:%dkbps duration:%s language:%#v",
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

	tgvideoFilename := fmt.Sprintf("%s.%s.mp4", tsversion(), v.Id)
	tgvideoFile, err := os.OpenFile(tgvideoFilename, os.O_RDWR|os.O_CREATE, 0600)
	if err != nil {
		return fmt.Errorf("Create file: %w", err)
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
		return fmt.Errorf("Download from youtube: %w", err)
	}

	if err := ytstream.Close(); err != nil {
		log("ytstream.Close: %v", err)
	}
	if err := tgvideoFile.Close(); err != nil {
		return fmt.Errorf("os.File.Close: %w", err)
	}

	log("Downloaded video in %v", time.Since(t0).Truncate(time.Second))
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

	if targetVideoBitrateKbps > 0 {
		log("Transcoding to audio:%dkbps video:%dkbps", TgAudioBitrateKbps, targetVideoBitrateKbps)
		tgvideoTranscodedFilename := fmt.Sprintf("%s.%s.%dk.mp4", tsversion(), v.Id, targetVideoBitrateKbps)
		ffmpegArgs := FfmpegGlobalOptions
		ffmpegArgs = append(ffmpegArgs,
			"-i", tgvideoFilename,
			"-f", "mp4",
			"-c:a", "aac",
			"-b:a", fmt.Sprintf("%dk", TgAudioBitrateKbps),
			"-c:v", "h264",
			"-b:v", fmt.Sprintf("%dk", targetVideoBitrateKbps),
			tgvideoTranscodedFilename,
		)
		ffmpegCmd := exec.Command(FfmpegPath, ffmpegArgs...)

		ffmpegCmdStderrPipe, err := ffmpegCmd.StderrPipe()
		if err != nil {
			return fmt.Errorf("Ffmpeg StderrPipe: %w", err)
		}

		t0 := time.Now()
		err = ffmpegCmd.Start()
		if err != nil {
			return fmt.Errorf("Ffmpeg Start: %w", err)
		}

		log("Started command `%s`", ffmpegCmd.String())

		_, err = io.Copy(os.Stderr, ffmpegCmdStderrPipe)
		if err != nil {
			log("Copy from ffmpeg stderr: %w", err)
		}

		err = ffmpegCmd.Wait()
		if err != nil {
			return fmt.Errorf("Ffmpeg Wait: %w", err)
		}

		log("Transcoded video in %v", time.Since(t0).Truncate(time.Second))

		if err := os.Remove(tgvideoFilename); err != nil {
			log("Remove: %v", err)
		}

		tgvideoCaption += NL + fmt.Sprintf("(transcoded to audio:%dkbps video:%dkbps)", TgAudioBitrateKbps, targetVideoBitrateKbps)
		tgvideoFilename = tgvideoTranscodedFilename
	}

	tgvideoReader, err := os.Open(tgvideoFilename)
	if err != nil {
		return fmt.Errorf("Open file: %w", err)
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
		log("Remove: %v", err)
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
		log("format: ContentLength:%dMB Language:%#v", f.ContentLength>>20, flang)
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
		"Downloading youtube audio size:%dMB bitrate:%dkbps duration:%s language:%#v",
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

	tgaudioFilename := fmt.Sprintf("%s.%s.m4a", tsversion(), v.Id)
	tgaudioFile, err := os.OpenFile(tgaudioFilename, os.O_RDWR|os.O_CREATE, 0600)
	if err != nil {
		return fmt.Errorf("Create file: %w", err)
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
		return fmt.Errorf("Download from youtube: %w", err)
	}

	if err := ytstream.Close(); err != nil {
		log("ytstream.Close: %v", err)
	}
	if err := tgaudioFile.Close(); err != nil {
		return fmt.Errorf("os.File.Close: %w", err)
	}

	log("Downloaded audio in %v", time.Since(t0).Truncate(time.Second))
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

	if targetAudioBitrateKbps > 0 {
		log("Transcoding to audio:%dkbps", targetAudioBitrateKbps)
		tgaudioTranscodedFilename := fmt.Sprintf("%s.%s.%dk.m4a", tsversion(), v.Id, targetAudioBitrateKbps)
		ffmpegArgs := FfmpegGlobalOptions
		ffmpegArgs = append(ffmpegArgs,
			"-i", tgaudioFilename,
			"-f", "mp4",
			"-c:a", "aac",
			"-b:a", fmt.Sprintf("%dk", targetAudioBitrateKbps),
			tgaudioTranscodedFilename,
		)
		ffmpegCmd := exec.Command(FfmpegPath, ffmpegArgs...)

		ffmpegCmdStderrPipe, err := ffmpegCmd.StderrPipe()
		if err != nil {
			return fmt.Errorf("Ffmpeg StderrPipe: %w", err)
		}

		t0 := time.Now()
		err = ffmpegCmd.Start()
		if err != nil {
			return fmt.Errorf("Ffmpeg Start: %w", err)
		}

		log("Started command `%s`", ffmpegCmd.String())

		_, err = io.Copy(os.Stderr, ffmpegCmdStderrPipe)
		if err != nil {
			log("Copy from ffmpeg stderr: %v", err)
		}

		err = ffmpegCmd.Wait()
		if err != nil {
			return fmt.Errorf("Ffmpeg Wait: %w", err)
		}

		log("Transcoded audio in %v", time.Since(t0).Truncate(time.Second))

		if err := os.Remove(tgaudioFilename); err != nil {
			log("Remove: %v", err)
		}

		tgaudioCaption += NL + fmt.Sprintf("(transcoded to %dkbps)", targetAudioBitrateKbps)
		tgaudioFilename = tgaudioTranscodedFilename
	}

	tgaudioReader, err := os.Open(tgaudioFilename)
	if err != nil {
		return fmt.Errorf("Open file: %w", err)
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
		log("Remove: %v", err)
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

	log("Playlist Title: %s", playlists.Items[0].Snippet.Title)
	log("Playlist Item Count: %d", playlists.Items[0].ContentDetails.ItemCount)

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
		defer log("mparterr: %v", err)

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

	resp, err := HttpClient.Post(
		fmt.Sprintf("https://api.telegram.org/bot%s/sendVideo", TgToken),
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

	return tgvideo, nil
}

func tgsendAudioFile(chatid int64, caption string, audio io.Reader, performer, title string, duration time.Duration) (tgaudio *TgAudio, err error) {
	piper, pipew := io.Pipe()
	mpartw := multipart.NewWriter(pipew)

	var mparterr error
	go func(err error) {
		defer log("mparterr: %v", err)

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

	resp, err := HttpClient.Post(
		fmt.Sprintf("https://api.telegram.org/bot%s/sendAudio", TgToken),
		mpartw.FormDataContentType(),
		piper,
	)
	if err != nil {
		if regexp.MustCompile("Too Many Requests: retry after [0-9]+$").MatchString(fmt.Sprintf("%s", err)) {
			log("telegram api too many requests: sleeping 33 seconds")
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

	return tgaudio, nil
}

func tgsendMessage(text string, chatid int64, parsemode string, replytomessageid int64) (msg *TgMessage, err error) {
	// https://core.telegram.org/bots/api/#sendmessage
	// https://core.telegram.org/bots/api/#formatting-options
	if parsemode == "MarkdownV2" {
		for _, c := range []string{`[`, `]`, `(`, `)`, `~`, "`", `>`, `#`, `+`, `-`, `=`, `|`, `{`, `}`, `.`, `!`} {
			text = strings.ReplaceAll(text, c, `\`+c)
		}
		text = strings.ReplaceAll(text, "______", `\_\_\_\_\_\_`)
		text = strings.ReplaceAll(text, "_____", `\_\_\_\_\_`)
		text = strings.ReplaceAll(text, "____", `\_\_\_\_`)
		text = strings.ReplaceAll(text, "___", `\_\_\_`)
		text = strings.ReplaceAll(text, "__", `\_\_`)
	}
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
		fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", TgToken),
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
		fmt.Sprintf("https://api.telegram.org/bot%s/deleteMessage", TgToken),
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
