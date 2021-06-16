/*
go get -u -v github.com/kkdai/youtube/v2

https://pkg.go.dev/github.com/kkdai/youtube/v2/
https://core.telegram.org/bots/api/

GoFmt GoBuildNull GoPublish

heroku git:clone -a tgzebot $HOME/tgzebot/
heroku buildpacks:set https://github.com/ryandotsmith/null-buildpack.git
heroku addons:create scheduler:standard
heroku addons:attach scheduler-xyz
GOOS=linux GOARCH=amd64 go build -trimpath -o $HOME/tgzebot/ && cp tgzebot.go $HOME/tgzebot/
cd $HOME/tgzebot/ && git commit -am tgzebot && git reset `{git commit-tree 'HEAD^{tree}' -m 'tgzebot'} && git push -f
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
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"

	dotenv "github.com/joho/godotenv"
	yt "github.com/kkdai/youtube/v2"
)

func beats(td time.Duration) int {
	const beat = time.Duration(24) * time.Hour / 1000
	return int(td / beat)
}

func ts() string {
	tzBiel := time.FixedZone("Biel", 60*60)
	t := time.Now().In(tzBiel)
	ts := fmt.Sprintf("%03d/%s@%d", t.Year()%1000, t.Format("0102"), beats(time.Since(time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, tzBiel))))
	return ts
}

func tsversion() string {
	tzBiel := time.FixedZone("Biel", 60*60)
	t := time.Now().In(tzBiel)
	v := fmt.Sprintf("%03d.%s.%d", t.Year()%1000, t.Format("0102"), beats(time.Since(time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, tzBiel))))
	return v
}

func log(msg interface{}, args ...interface{}) {
	fmt.Fprintf(os.Stderr, ts()+" "+fmt.Sprintf("%s", msg)+NL, args...)
}

const (
	NL = "\n"

	DotenvPath = "tgzebot.env"

	TgMaxFileSize      = 50 * 1000 * 1000
	TgAudioBitrateKbps = 50

	// https://golang.org/s/re2syntax
	// (?:re)	non-capturing group
	YtReString     = `(?:youtu.be/|youtube.com/watch\?v=)([0-9A-Za-z_-]+)`
	YtListReString = `youtube.com/.*[?&]list=([0-9A-Za-z_-]+)`
)

var (
	Ctx        context.Context
	HttpClient = &http.Client{}
	YtCl       yt.Client

	ytRe, ytlistRe *regexp.Regexp

	YtMaxResults = 50
	YtKey        string

	TgToken    string
	TgOffset   int64
	ZeTgChatId int64

	FfmpegPath string = "./ffmpeg"

	HerokuToken   string
	HerokuVarsUrl string

	TgQuest1    string
	TgQuest1Key string
	TgQuest2    string
	TgQuest2Key string
	TgQuest3    string
	TgQuest3Key string
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
	UpdateId          int64     `json:"update_id"`
	Message           TgMessage `json:"message"`
	EditedMessage     TgMessage `json:"edited_message"`
	ChannelPost       TgMessage `json:"channel_post"`
	EditedChannelPost TgMessage `json:"edited_channel_post"`
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

func Setenv(name, value string) error {
	if HerokuVarsUrl != "" && HerokuToken != "" {
		return HerokuSetenv(name, value)
	}

	env, err := dotenv.Read(DotenvPath)
	if err != nil {
		log("WARNING: loading dotenv file: %v", err)
		env = make(map[string]string)
	}
	env[name] = value
	if err = dotenv.Write(env, DotenvPath); err != nil {
		return err
	}

	return nil
}

func HerokuSetenv(name, value string) error {
	req, err := http.NewRequest(
		"PATCH",
		HerokuVarsUrl,
		strings.NewReader(fmt.Sprintf(`{"%s": "%s"}`, name, value)),
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
	ytRe = regexp.MustCompile(YtReString)
	ytlistRe = regexp.MustCompile(YtListReString)

	if err = dotenv.Overload(DotenvPath); err != nil {
		log("WARNING: loading dotenv file: %v", err)
	}

	if os.Getenv("TgToken") != "" {
		TgToken = os.Getenv("TgToken")
	}
	if TgToken == "" {
		log("ERROR: TgToken empty")
		os.Exit(1)
	}
	if os.Getenv("TgOffset") != "" {
		TgOffset, err = strconv.ParseInt(os.Getenv("TgOffset"), 10, 0)
		if err != nil {
			log("ERROR: invalid TgOffset: %v", err)
			os.Exit(1)
		}
	}
	if os.Getenv("ZeTgChatId") == "" {
		log("ERROR: ZeTgChatId empty")
		os.Exit(1)
	} else {
		ZeTgChatId, err = strconv.ParseInt(os.Getenv("ZeTgChatId"), 10, 0)
		if err != nil {
			log("ERROR: invalid ZeTgChatId: %v", err)
			os.Exit(1)
		}
	}

	TgQuest1 = os.Getenv("TgQuest1")
	TgQuest1Key = os.Getenv("TgQuest1Key")
	TgQuest2 = os.Getenv("TgQuest2")
	TgQuest2Key = os.Getenv("TgQuest2Key")
	TgQuest3 = os.Getenv("TgQuest3")
	TgQuest3Key = os.Getenv("TgQuest3Key")

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

	HerokuToken = os.Getenv("HerokuToken")
	if HerokuToken == "" {
		log("WARNING: HerokuToken empty")
	}
	HerokuVarsUrl = os.Getenv("HerokuVarsUrl")
	if HerokuVarsUrl == "" {
		log("WARNING: HerokuVarsUrl empty")
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
		return nil, fmt.Errorf("Tg response not ok: %s", tgResp.Description)
	}

	return tgResp.Result, nil
}

func tggetChatAdministrators(chatid int64) (mm []TgChatMember, err error) {
	getChatAdministratorsUrl := fmt.Sprintf("https://api.telegram.org/bot%s/getChatAdministrators?chat_id=%d", TgToken, chatid)
	var tgResp TgGetChatAdministratorsResponse

	err = getJson(getChatAdministratorsUrl, &tgResp)
	if err != nil {
		return nil, err
	}
	if !tgResp.Ok {
		return nil, fmt.Errorf("Tg response not ok: %s", tgResp.Description)
	}

	return tgResp.Result, nil
}

type YtVideo struct {
	Id string
}

func main() {
	var err error
	var uu []TgUpdate
	uu, err = tggetUpdates()
	if err != nil {
		log("tggetUpdates: %v", err)
		os.Exit(1)
	}

	var m, prevm TgMessage
	for _, u := range uu {
		var iseditmessage bool
		var ischannelpost bool
		if u.Message.MessageId != 0 {
			m = u.Message
		} else if u.EditedMessage.MessageId != 0 {
			m = u.EditedMessage
			iseditmessage = true
		} else if u.ChannelPost.MessageId != 0 {
			m = u.ChannelPost
		} else if u.EditedChannelPost.MessageId != 0 {
			m = u.EditedChannelPost
			iseditmessage = true
		} else {
			log("Unsupported type of update received")
			_, err = tgsendMessage("Unsupported type of update received", ZeTgChatId, "")
			if err != nil {
				log("tgsendMessage: %v", err)
				continue
			}
			continue
		}

		if m.Chat.Type == "channel" {
			ischannelpost = true
		}

		log("Message text: `%s`", m.Text)

		shouldreport := true
		if m.From.Id == ZeTgChatId {
			shouldreport = false
		}
		var chatadmins string
		if aa, err := tggetChatAdministrators(m.Chat.Id); err == nil {
			for _, a := range aa {
				chatadmins += fmt.Sprintf("@%s;id:%d;status:%s ", a.User.Username, a.User.Id, a.Status)
				if a.User.Id == ZeTgChatId {
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
			_, err = tgsendMessage(report, ZeTgChatId, "")
			if err != nil {
				log("tgsendMessage: %v", err)
				continue
			}
		}

		if strings.TrimSpace(m.Text) == TgQuest1 {
			_, err = tgsendMessage(TgQuest1Key, m.Chat.Id, "")
			if err != nil {
				log("tgsendMessage: %v", err)
			}
		}
		if strings.TrimSpace(m.Text) == TgQuest2 {
			_, err = tgsendMessage(TgQuest2Key, m.Chat.Id, "")
			if err != nil {
				log("tgsendMessage: %v", err)
			}
		}
		if strings.TrimSpace(m.Text) == TgQuest3 {
			_, err = tgsendMessage(TgQuest3Key, m.Chat.Id, "")
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
			var vinfo *yt.Video
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
			}

			if postingerr == nil {
				if ischannelpost {
					err = tgdeleteMessage(m.Chat.Id, m.MessageId)
					if err != nil {
						log("tgdeleteMessage: %v", err)
					}
				}
			} else {
				_, err = tgsendMessage(fmt.Sprintf("%s\nError: %v", m.Text, postingerr), m.Chat.Id, "")
				if err != nil {
					log("tgsendMessage: %v", err)
				}
			}

		}

		if TgOffset <= u.UpdateId {
			TgOffset = u.UpdateId + 1
			err = Setenv("TgOffset", fmt.Sprintf("%d", TgOffset))
			if err != nil {
				log("Setenv TgOffset: %v", err)
				os.Exit(1)
			}
		}
	}

	return
}

func postVideo(v YtVideo, vinfo *yt.Video, m TgMessage) error {
	var videoFormat, videoSmallestFormat yt.Format

	for _, f := range vinfo.Formats {
		if !strings.Contains(f.MimeType, "/mp4") {
			continue
		}
		fsize := f.ContentLength
		if fsize == 0 {
			fsize = int64(f.Bitrate / 8 * int(vinfo.Duration.Seconds()))
		}
		if strings.HasPrefix(f.MimeType, "video/mp4") && f.QualityLabel != "" && f.AudioQuality != "" {
			if videoSmallestFormat.ItagNo == 0 || f.Bitrate < videoSmallestFormat.Bitrate {
				videoSmallestFormat = f
			}
			if fsize < TgMaxFileSize && f.Bitrate > videoFormat.Bitrate {
				videoFormat = f
			}
		}
	}

	var targetVideoBitrateKbps int64
	if videoFormat.ItagNo == 0 {
		videoFormat = videoSmallestFormat
		targetVideoSize := int64(TgMaxFileSize - (TgAudioBitrateKbps*1024*int64(vinfo.Duration.Seconds()+1))/8)
		targetVideoBitrateKbps = int64(((targetVideoSize * 8) / int64(vinfo.Duration.Seconds()+1)) / 1024)
	}

	ytstream, ytstreamsize, err := YtCl.GetStreamContext(Ctx, vinfo, &videoFormat)
	if err != nil {
		return fmt.Errorf("GetStreamContext: %v", err)
	}
	defer ytstream.Close()

	log(
		"Youtube video size:%dmb quality:%s bitrate:%dkbps duration:%ds",
		ytstreamsize/1000/1000,
		videoFormat.QualityLabel,
		videoFormat.Bitrate/1024,
		int64(vinfo.Duration.Seconds()),
	)

	var tgvideo *TgVideo
	tgvideoCaption := fmt.Sprintf("%s\n"+"https://youtu.be/%s %s", vinfo.Title, v.Id, videoFormat.QualityLabel)

	tgvideoFilename := fmt.Sprintf("%s.%s.mp4", tsversion(), v.Id)
	tgvideoFile, err := os.OpenFile(tgvideoFilename, os.O_RDWR|os.O_CREATE, 0600)
	if err != nil {
		return fmt.Errorf("Create file: %v", err)
	}
	t0 := time.Now()
	_, err = io.Copy(tgvideoFile, ytstream)
	if err != nil {
		return fmt.Errorf("Download from youtube: %v", err)
	}
	log("Downloaded video in %ds", int64(time.Since(t0).Seconds()))
	if err := ytstream.Close(); err != nil {
		log("ytstream.Close: %v", err)
	}
	if err := tgvideoFile.Close(); err != nil {
		return fmt.Errorf("os.File.Close: %v", err)
	}

	if targetVideoBitrateKbps > 0 {
		log("Transcoding video to %dkbps", targetVideoBitrateKbps)
		transcodingmessage, err := tgsendMessage(fmt.Sprintf("%s"+NL+"https://youtu.be/%s"+NL+"transcoding to audio:%dkbps, video:%dkbps", vinfo.Title, v.Id, TgAudioBitrateKbps, targetVideoBitrateKbps), m.Chat.Id, "")
		if err == nil && transcodingmessage != nil {
			defer tgdeleteMessage(m.Chat.Id, transcodingmessage.MessageId)
		}

		tgvideoTranscodedFilename := fmt.Sprintf("%s.%s.%dk.mp4", tsversion(), v.Id, targetVideoBitrateKbps)
		ffmpegCmd := exec.Command(
			FfmpegPath,
			"-i", tgvideoFilename,
			"-f", "mp4",
			"-c:a", "aac",
			"-b:a", fmt.Sprintf("%dk", TgAudioBitrateKbps),
			"-c:v", "h264",
			"-b:v", fmt.Sprintf("%dk", targetVideoBitrateKbps),
			tgvideoTranscodedFilename,
		)

		ffmpegCmdStderrPipe, err := ffmpegCmd.StderrPipe()
		if err != nil {
			return fmt.Errorf("Ffmpeg StderrPipe: %v", err)
		}

		t0 := time.Now()
		err = ffmpegCmd.Start()
		if err != nil {
			return fmt.Errorf("Ffmpeg Start: %v", err)
		}

		log("Started command `%s`", ffmpegCmd.String())

		_, err = io.Copy(os.Stderr, ffmpegCmdStderrPipe)
		if err != nil {
			log("Copy from ffmpeg stderr: %v", err)
		}

		err = ffmpegCmd.Wait()
		if err != nil {
			return fmt.Errorf("Ffmpeg Wait: %v", err)
		}

		log("Transcoded video in %ds", int64(time.Since(t0).Seconds()))

		if err := os.Remove(tgvideoFilename); err != nil {
			log("Remove: %v", err)
		}

		tgvideoCaption += NL + fmt.Sprintf("(transcoded to audio:%dkbps video:%dkbps)", TgAudioBitrateKbps, targetVideoBitrateKbps)
		tgvideoFilename = tgvideoTranscodedFilename
	}

	tgvideoReader, err := os.Open(tgvideoFilename)
	if err != nil {
		return fmt.Errorf("Open file: %v", err)
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
		return fmt.Errorf("tgsendVideoFile: %v", err)
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

func postAudio(v YtVideo, vinfo *yt.Video, m TgMessage) error {
	var audioFormat, audioSmallestFormat yt.Format

	for _, f := range vinfo.Formats {
		if !strings.Contains(f.MimeType, "/mp4") {
			continue
		}
		fsize := f.ContentLength
		if fsize == 0 {
			fsize = int64(f.Bitrate / 8 * int(vinfo.Duration.Seconds()))
		}
		if strings.HasPrefix(f.MimeType, "audio/mp4") {
			if audioSmallestFormat.ItagNo == 0 || f.Bitrate < audioSmallestFormat.Bitrate {
				audioSmallestFormat = f
			}
			if fsize < TgMaxFileSize && f.Bitrate > audioFormat.Bitrate {
				audioFormat = f
			}
		}
	}

	var targetAudioBitrateKbps int64
	if audioFormat.ItagNo == 0 {
		audioFormat = audioSmallestFormat
		targetAudioBitrateKbps = int64(((TgMaxFileSize * 8) / int64(vinfo.Duration.Seconds()+1)) / 1024)
	}

	ytstream, ytstreamsize, err := YtCl.GetStreamContext(Ctx, vinfo, &audioFormat)
	if err != nil {
		return fmt.Errorf("GetStreamContext: %v", err)
	}
	defer ytstream.Close()

	log(
		"Downloading youtube audio size:%dmb bitrate:%dkbps duration:%ds",
		ytstreamsize/1000/1000,
		audioFormat.Bitrate/1024,
		int64(vinfo.Duration.Seconds()),
	)

	var tgaudio *TgAudio
	tgaudioCaption := fmt.Sprintf("https://youtu.be/%s %dkbps", v.Id, audioFormat.Bitrate/1024)

	tgaudioFilename := fmt.Sprintf("%s.%s.m4a", tsversion(), v.Id)
	tgaudioFile, err := os.OpenFile(tgaudioFilename, os.O_RDWR|os.O_CREATE, 0600)
	if err != nil {
		return fmt.Errorf("Create file: %v", err)
	}
	t0 := time.Now()
	_, err = io.Copy(tgaudioFile, ytstream)
	if err != nil {
		return fmt.Errorf("Download from youtube: %v", err)
	}
	log("Downloaded audio in %ds", int64(time.Since(t0).Seconds()))
	if err := ytstream.Close(); err != nil {
		log("ytstream.Close: %v", err)
	}
	if err := tgaudioFile.Close(); err != nil {
		return fmt.Errorf("os.File.Close: %v", err)
	}

	if targetAudioBitrateKbps > 0 {
		log("Transcoding audio to %dkbps", targetAudioBitrateKbps)
		transcodingmessage, err := tgsendMessage(fmt.Sprintf("https://youtu.be/%s"+NL+"transcoding to audio:%dkbps", v.Id, targetAudioBitrateKbps), m.Chat.Id, "")
		if err == nil && transcodingmessage != nil {
			defer tgdeleteMessage(m.Chat.Id, transcodingmessage.MessageId)
		}

		tgaudioTranscodedFilename := fmt.Sprintf("%s.%s.%dk.m4a", tsversion(), v.Id, targetAudioBitrateKbps)
		ffmpegCmd := exec.Command(
			FfmpegPath,
			"-i", tgaudioFilename,
			"-f", "mp4",
			"-c:a", "aac",
			"-b:a", fmt.Sprintf("%dk", targetAudioBitrateKbps),
			tgaudioTranscodedFilename,
		)

		ffmpegCmdStderrPipe, err := ffmpegCmd.StderrPipe()
		if err != nil {
			return fmt.Errorf("Ffmpeg StderrPipe: %v", err)
		}

		t0 := time.Now()
		err = ffmpegCmd.Start()
		if err != nil {
			return fmt.Errorf("Ffmpeg Start: %v", err)
		}

		log("Started command `%s`", ffmpegCmd.String())

		_, err = io.Copy(os.Stderr, ffmpegCmdStderrPipe)
		if err != nil {
			log("Copy from ffmpeg stderr: %v", err)
		}

		err = ffmpegCmd.Wait()
		if err != nil {
			return fmt.Errorf("Ffmpeg Wait: %v", err)
		}

		log("Transcoded audio in %ds", int64(time.Since(t0).Seconds()))

		if err := os.Remove(tgaudioFilename); err != nil {
			log("Remove: %v", err)
		}

		tgaudioCaption += NL + fmt.Sprintf("(transcoded to %dkbps)", targetAudioBitrateKbps)
		tgaudioFilename = tgaudioTranscodedFilename
	}

	tgaudioReader, err := os.Open(tgaudioFilename)
	if err != nil {
		return fmt.Errorf("Open file: %v", err)
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
		return fmt.Errorf("tgsendAudioFile: %v", err)
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

		if err := mpartw.Close(); err != nil {
			err = fmt.Errorf("multipart.Writer.Close: %v", err)
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

		if err := mpartw.Close(); err != nil {
			err = fmt.Errorf("multipart.Writer.Close: %v", err)
			return
		}
	}(mparterr)

	resp, err := HttpClient.Post(
		fmt.Sprintf("https://api.telegram.org/bot%s/sendAudio", TgToken),
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

func tgsendMessage(text string, chatid int64, parsemode string) (msg *TgMessage, err error) {
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
		return fmt.Errorf("postJson: %v", err)
	}

	if !tgresp.Ok {
		return fmt.Errorf("deleteMessage: %s", tgresp.Description)
	}

	return nil
}
