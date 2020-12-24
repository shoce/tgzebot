/*
go get -u -v github.com/kkdai/youtube/...

https://github.com/kkdai/youtube/v2
https://core.telegram.org/bots/api

GoFmt GoBuildNull GoPublish

heroku git:clone -a tgzebot $HOME/tgzebot/
heroku buildpacks:set https://github.com/ryandotsmith/null-buildpack.git
GOOS=linux GOARCH=amd64 go build -trimpath -o $HOME/tgzebot/
cp tgzebot.go $HOME/tgzebot/
cd $HOME/tgzebot/
git commit -am tgzebot
git reset $(git commit-tree HEAD^{tree} -m "tgzebot")
git push -f
*/

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"

	yt "github.com/kkdai/youtube"
)

func beats(td time.Duration) int {
	const beat = time.Duration(24) * time.Hour / 1000
	return int(td / beat)
}

func ts() string {
	t := time.Now().Local()
	ts := fmt.Sprintf("%s@%d", t.Format("0102"), beats(time.Since(time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.Local))))
	return ts
}

func log(msg interface{}, args ...interface{}) {
	fmt.Fprintf(os.Stderr, ts()+" "+fmt.Sprintf("%s", msg)+"\n", args...)
}

const (
	TgMaxFileSize = 50 * 1000 * 1000

	YoutubeREString = `youtube.com/watch\?v=([0-9A-Za-z_-]+)`
	YoutuREString   = `youtu.be/([0-9A-Za-z_-]+)`
	YtListReString  = `youtube.com/.*[?&]list=([0-9A-Za-z_-]+)`
)

var (
	Ctx        context.Context
	HttpClient = &http.Client{}
	YtCl       yt.Client

	youtubeRe, youtuRe, ytListRe *regexp.Regexp

	YtMaxResults = 50
	YtKey        string

	TgToken  string
	TgChatId int64
	TgOffset int64

	FfmpegPath string = "./ffmpeg"

	HerokuToken   string
	HerokuVarsUrl string

	TgQuest    string
	TgQuestKey string
)

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

type YtPlaylistItemSnippet struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	PublishedAt string `json:"publishedAt"`
	Thumbnails  struct {
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
	UpdateId    int64     `json:"update_id"`
	Message     TgMessage `json:"message"`
	ChannelPost TgMessage `json:"channel_post"`
}

func getJson(url string, target interface{}) error {
	r, err := HttpClient.Get(url)
	if err != nil {
		return err
	}
	defer r.Body.Close()

	return json.NewDecoder(r.Body).Decode(target)
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
		return fmt.Errorf("io.Copy: %v", err)
	}

	err = json.NewDecoder(respBody).Decode(target)
	if err != nil {
		return fmt.Errorf("Decode: %v", err)
	}

	return nil
}

func HerokuUpdateTgOffset() error {
	req, err := http.NewRequest(
		"PATCH",
		HerokuVarsUrl,
		strings.NewReader(fmt.Sprintf(`{"TgOffset": "%d"}`, TgOffset)),
	)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/vnd.heroku+json; version=3")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", HerokuToken))
	req.Header.Set("Content-Type", "application/json")
	resp, err := HttpClient.Do(req)
	if err != nil {
		return err
	}
	if resp.StatusCode != 200 {
		return fmt.Errorf("response status: %s", resp.Status)
	}
	return nil
}

func init() {
	var err error

	Ctx = context.TODO()
	YtCl = yt.Client{HTTPClient: &http.Client{}}
	youtubeRe = regexp.MustCompile(YoutubeREString)
	youtuRe = regexp.MustCompile(YoutuREString)
	ytListRe = regexp.MustCompile(YtListReString)

	if os.Getenv("TgToken") != "" {
		TgToken = os.Getenv("TgToken")
	}
	if TgToken == "" {
		log("ERROR: TgToken empty")
		os.Exit(1)
	}
	if os.Getenv("TgChatId") == "" {
		log("ERROR: TgChatId empty")
		os.Exit(1)
	} else {
		TgChatId, err = strconv.ParseInt(os.Getenv("TgChatId"), 10, 0)
		if err != nil {
			log("ERROR: invalid TgChatId: %v", err)
			os.Exit(1)
		}
	}
	if os.Getenv("TgOffset") != "" {
		TgOffset, err = strconv.ParseInt(os.Getenv("TgOffset"), 10, 0)
		if err != nil {
			log("ERROR: invalid TgOffset: %v", err)
			os.Exit(1)
		}
	}

	if os.Getenv("TgQuest") != "" {
		TgQuest = os.Getenv("TgQuest")
	}
	if os.Getenv("TgQuestKey") != "" {
		TgQuestKey = os.Getenv("TgQuestKey")
	}

	if os.Getenv("YtKey") != "" {
		YtKey = os.Getenv("YtKey")
	}
	if YtKey == "" {
		log("ERROR: YtKey empty")
		os.Exit(1)
	}

	if os.Getenv("FfmpegPath") != "" {
		FfmpegPath = os.Getenv("FfmpegPath")
	}

	if os.Getenv("HerokuToken") != "" {
		HerokuToken = os.Getenv("HerokuToken")
	}
	if HerokuToken == "" {
		log("ERROR: HerokuToken empty")
		os.Exit(1)
	}
	if os.Getenv("HerokuVarsUrl") != "" {
		HerokuVarsUrl = os.Getenv("HerokuVarsUrl")
	}
	if HerokuVarsUrl == "" {
		log("ERROR: HerokuVarsUrl empty")
		os.Exit(1)
	}
}

func tggetUpdates() (uu []TgUpdate, err error) {
	getUpdatesUrl := fmt.Sprintf("https://api.telegram.org/bot%s/getUpdates?offset=%d", TgToken, TgOffset)
	var tgResp TgGetUpdatesResponse

	err = getJson(getUpdatesUrl, &tgResp)
	if err != nil {
		return nil, err
	}
	if !tgResp.Ok {
		return nil, fmt.Errorf("Tg response not Ok: %s", tgResp.Description)
	}

	return tgResp.Result, nil
}

type YtVideo struct {
	Id string
}

func main() {
	var err error

	uu, err := tggetUpdates()
	if err != nil {
		log("tggetUpdates: %v", err)
		os.Exit(1)
	}

	var m, prevm TgMessage
	for _, u := range uu {
		if u.Message.MessageId != 0 {
			m = u.Message
		} else if u.ChannelPost.MessageId != 0 {
			m = u.ChannelPost
		}
		log("Message text: `%s`", m.Text)

		if m.From.Id != TgChatId {
			report := fmt.Sprintf(
				"%s"+"\n"+
					"from: @%s id%d %s %s"+"\n"+
					"chat: id%d %s %s %s",
				m.Text,
				m.From.Username, m.From.Id, m.From.FirstName, m.From.LastName,
				m.Chat.Id, m.Chat.Type, m.Chat.Title, m.Chat.InviteLink,
			)
			_, err = tgsendMessage(report, TgChatId)
			if err != nil {
				log("tgsendMessage: %v", err)
				continue
			}
		}

		if strings.TrimSpace(m.Text) == TgQuest {
			_, err = tgsendMessage(TgQuestKey, m.Chat.Id)
			if err != nil {
				log("tgsendMessage: %v", err)
			}
		}

		var downloadvideo bool
		if strings.HasPrefix(strings.ToLower(m.Text), "video ") || strings.HasSuffix(strings.ToLower(m.Text), " video") || strings.ToLower(prevm.Text) == "video" {
			downloadvideo = true
		}
		prevm = m

		var videos []YtVideo
		var mm []string

		if mm = ytListRe.FindStringSubmatch(m.Text); len(mm) > 1 {
			videos, err = getList(mm[1])
			if err != nil {
				log("%v", err)
				continue
			}
		} else if mm = youtubeRe.FindStringSubmatch(m.Text); len(mm) > 1 {
			videos = []YtVideo{YtVideo{Id: mm[1]}}
		} else if mm = youtuRe.FindStringSubmatch(m.Text); len(mm) > 1 {
			videos = []YtVideo{YtVideo{Id: mm[1]}}
		}

		for _, v := range videos {
			vinfo, err := YtCl.GetVideoContext(Ctx, v.Id)
			if err != nil {
				log("%v", err)
				continue
			}

			var videoFormat yt.Format
			var audioFormat yt.Format
			for _, f := range vinfo.Formats {
				fsize, _ := strconv.ParseInt(f.ContentLength, 10, 64)
				if fsize == 0 {
					fsize = int64(f.Bitrate / 8 * int(vinfo.Duration.Seconds()))
				}
				if fsize > TgMaxFileSize {
					continue
				}
				log(
					"mimetype:`%s` qualitylabel:`%s` audioquality:`%s` bitrate:%dkbps size:%dmb",
					f.MimeType, f.QualityLabel, f.AudioQuality, f.Bitrate/1024, fsize/1000/1000,
				)
				if strings.HasPrefix(f.MimeType, "video/mp4") && f.QualityLabel != "" && f.AudioQuality != "" && f.Bitrate > videoFormat.Bitrate {
					log("Choosing this video format")
					videoFormat = f
				}
				if strings.HasPrefix(f.MimeType, "audio/mp4") && f.Bitrate > audioFormat.Bitrate {
					log("Choosing this audio format")
					audioFormat = f
				}
			}

			if downloadvideo {
				if videoFormat.ItagNo == 0 {
					log("No suitable video format found")
					_, err = tgsendMessage(fmt.Sprintf("youtu.be/%s no suitable video format found", v.Id), m.Chat.Id)
					if err != nil {
						log("tgsendMessage: %v", err)
					}
					continue
				}

				resp, err := YtCl.GetStreamContext(Ctx, vinfo, &videoFormat)
				if err != nil {
					log("GetStreamContext: %v", err)
					continue
				}
				defer resp.Body.Close()

				log(
					"Youtube video size:%dmb quality:%s bitrate:%dkbps duration:%ds",
					resp.ContentLength/1000/1000,
					videoFormat.QualityLabel,
					videoFormat.Bitrate/1024,
					int64(vinfo.Duration.Seconds()),
				)
				tgvideo, err := tgsendVideoFile(
					m.Chat.Id,
					fmt.Sprintf("%s\nhttps://youtu.be/%s %s", vinfo.Title, v.Id, videoFormat.QualityLabel),
					videoFormat.Width,
					videoFormat.Height,
					resp.Body,
					vinfo.Duration,
				)
				resp.Body.Close()
				if err != nil {
					log("tgsendVideoFile: %v", err)
					continue
				}
				if tgvideo.FileId == "" {
					log("tgsendVideoFile: file_id empty")
					continue
				}
			} else {
				if audioFormat.ItagNo == 0 {
					log("No suitable audio format found")
					_, err = tgsendMessage(fmt.Sprintf("youtu.be/%s no suitable audio format found", v.Id), m.Chat.Id)
					if err != nil {
						log("tgsendMessage: %v", err)
					}
					continue
				}

				resp, err := YtCl.GetStreamContext(Ctx, vinfo, &audioFormat)
				if err != nil {
					log("GetStreamContext: %v", err)
					continue
				}
				defer resp.Body.Close()

				log(
					"Youtube audio size:%dmb bitrate:%dkbps duration:%ds",
					resp.ContentLength/1000/1000,
					audioFormat.Bitrate/1024,
					int64(vinfo.Duration.Seconds()),
				)
				tgaudio, err := tgsendAudioFile(
					m.Chat.Id,
					vinfo.Author,
					vinfo.Title,
					fmt.Sprintf("https://youtu.be/%s %dkbps", v.Id, audioFormat.Bitrate/1024),
					resp.Body,
					vinfo.Duration,
				)
				resp.Body.Close()
				if err != nil {
					log("tgsendAudioFile: %v", err)
					continue
				}
				if tgaudio.FileId == "" {
					log("tgsendAudioFile: file_id empty")
					continue
				}
			}
		}

		if TgOffset <= u.UpdateId {
			TgOffset = u.UpdateId + 1
			err = HerokuUpdateTgOffset()
			if err != nil {
				log("HerokuUpdateTgOffset: %v", err)
				os.Exit(1)
			}
		}
	}

	return
}

func getList(ytlistid string) (ytitems []YtVideo, err error) {
	var videos []YtPlaylistItemSnippet
	nextPageToken := ""

	for nextPageToken != "" || len(videos) == 0 {
		var PlaylistItemsUrl = fmt.Sprintf("https://www.googleapis.com/youtube/v3/playlistItems?maxResults=%d&part=snippet&playlistId=%s&key=%s&pageToken=%s", YtMaxResults, ytlistid, YtKey, nextPageToken)

		var playlistItems YtPlaylistItems
		err = getJson(PlaylistItemsUrl, &playlistItems)
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
		ytitems = append(ytitems, YtVideo{Id: vid.ResourceId.VideoId})
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

func tgsendVideoFile(chatid int64, caption string, width, height int, video io.Reader, duration time.Duration) (tgvideo *TgVideo, err error) {
	piper, pipew := io.Pipe()
	mpartw := multipart.NewWriter(pipew)

	var mparterr error
	go func(err error) {
		var formw io.Writer

		defer pipew.Close()

		// chat_id
		formw, err = mpartw.CreateFormField("chat_id")
		if err != nil {
			err = fmt.Errorf("CreateFormField(`chat_id`): %v", err)
			return
		}
		_, err = formw.Write([]byte(strconv.Itoa(int(chatid))))
		if err != nil {
			err = fmt.Errorf("Write(chat_id): %v", err)
			return
		}

		// caption
		formw, err = mpartw.CreateFormField("caption")
		if err != nil {
			err = fmt.Errorf("CreateFormField(`caption`): %v", err)
			return
		}
		_, err = formw.Write([]byte(caption))
		if err != nil {
			err = fmt.Errorf("Write(caption): %v", err)
			return
		}

		// width
		formw, err = mpartw.CreateFormField("width")
		if err != nil {
			err = fmt.Errorf("CreateFormField(`width`): %v", err)
			return
		}
		_, err = formw.Write([]byte(strconv.Itoa(width)))
		if err != nil {
			err = fmt.Errorf("Write(width): %v", err)
			return
		}

		// height
		formw, err = mpartw.CreateFormField("height")
		if err != nil {
			err = fmt.Errorf("CreateFormField(`height`): %v", err)
			return
		}
		_, err = formw.Write([]byte(strconv.Itoa(height)))
		if err != nil {
			err = fmt.Errorf("Write(height): %v", err)
			return
		}

		// video
		formw, err = mpartw.CreateFormFile("video", safestring(caption))
		if err != nil {
			err = fmt.Errorf("CreateFormFile('video'): %v", err)
			return
		}
		_, err = io.Copy(formw, video)
		if err != nil {
			err = fmt.Errorf("Copy video: %v", err)
			return
		}

		// duration
		formw, err = mpartw.CreateFormField("duration")
		if err != nil {
			err = fmt.Errorf("CreateFormField(`duration`): %v", err)
			return
		}
		_, err = formw.Write([]byte(strconv.Itoa(int(duration.Seconds()))))
		if err != nil {
			err = fmt.Errorf("Write(duration): %v", err)
			return
		}

		err = mpartw.Close()
		if err != nil {
			err = fmt.Errorf("multipartWriter.Close: %v", err)
			return
		}
	}(mparterr)

	resp, err := HttpClient.Post(
		fmt.Sprintf("https://api.telegram.org/bot%s/sendVideo", TgToken),
		mpartw.FormDataContentType(),
		piper,
	)
	if err != nil {
		return nil, fmt.Errorf("Post: %v", err)
	}
	defer resp.Body.Close()

	if mparterr != nil {
		return nil, err
	}

	var tgresp TgResponse
	err = json.NewDecoder(resp.Body).Decode(&tgresp)
	if err != nil {
		return nil, fmt.Errorf("Decode: %v", err)
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

func tgsendAudioFile(chatid int64, performer, title, caption string, audio io.Reader, duration time.Duration) (tgaudio *TgAudio, err error) {
	piper, pipew := io.Pipe()
	mpartw := multipart.NewWriter(pipew)

	var mparterr error
	go func(err error) {
		var formw io.Writer

		defer pipew.Close()

		// chat_id
		formw, err = mpartw.CreateFormField("chat_id")
		if err != nil {
			err = fmt.Errorf("CreateFormField(`chat_id`): %v", err)
			return
		}
		_, err = formw.Write([]byte(strconv.Itoa(int(chatid))))
		if err != nil {
			err = fmt.Errorf("Write(chat_id): %v", err)
			return
		}

		// performer
		formw, err = mpartw.CreateFormField("performer")
		if err != nil {
			err = fmt.Errorf("CreateFormField(`performer`): %v", err)
			return
		}
		_, err = formw.Write([]byte(performer))
		if err != nil {
			err = fmt.Errorf("Write(performer): %v", err)
			return
		}

		// title
		formw, err = mpartw.CreateFormField("title")
		if err != nil {
			err = fmt.Errorf("CreateFormField(`title`): %v", err)
			return
		}
		_, err = formw.Write([]byte(title))
		if err != nil {
			err = fmt.Errorf("Write(title): %v", err)
			return
		}

		// caption
		formw, err = mpartw.CreateFormField("caption")
		if err != nil {
			err = fmt.Errorf("CreateFormField(`caption`): %v", err)
			return
		}
		_, err = formw.Write([]byte(caption))
		if err != nil {
			err = fmt.Errorf("Write(caption): %v", err)
			return
		}

		// audio
		formw, err = mpartw.CreateFormFile("audio", safestring(fmt.Sprintf("%s.%s", performer, title)))
		if err != nil {
			err = fmt.Errorf("CreateFormFile('audio'): %v", err)
			return
		}
		_, err = io.Copy(formw, audio)
		if err != nil {
			err = fmt.Errorf("Copy audio: %v", err)
			return
		}

		// duration
		formw, err = mpartw.CreateFormField("duration")
		if err != nil {
			err = fmt.Errorf("CreateFormField(`duration`): %v", err)
			return
		}
		_, err = formw.Write([]byte(strconv.Itoa(int(duration.Seconds()))))
		if err != nil {
			err = fmt.Errorf("Write(duration): %v", err)
			return
		}

		err = mpartw.Close()
		if err != nil {
			err = fmt.Errorf("multipartWriter.Close: %v", err)
			return
		}
	}(mparterr)

	resp, err := HttpClient.Post(
		fmt.Sprintf("https://api.telegram.org/bot%s/sendAudio", TgToken),
		mpartw.FormDataContentType(),
		piper,
	)
	if err != nil {
		return nil, fmt.Errorf("Post: %v", err)
	}
	defer resp.Body.Close()

	if mparterr != nil {
		return nil, err
	}

	var tgresp TgResponse
	err = json.NewDecoder(resp.Body).Decode(&tgresp)
	if err != nil {
		return nil, fmt.Errorf("Decode: %v", err)
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

func tgsendMessage(message string, chatid int64) (msg *TgMessage, err error) {
	sendMessage := map[string]interface{}{
		"chat_id":                  chatid,
		"text":                     message,
		"disable_web_page_preview": true,
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
