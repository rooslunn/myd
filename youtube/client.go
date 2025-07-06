package youtube

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"math/rand"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type clientInfo struct {
	name           string
	key            string
	version        string
	userAgent      string
	androidVersion int
	deviceModel    string
}

type Client struct {
	HTTPClient *http.Client
	client *clientInfo
	Info string
	consentID string
	visitorId struct {
		value   string
		updated time.Time
	}
}

var (
	IOSClient = clientInfo{
		name:        "IOS",
		version:     "19.45.4",
		key:         "AIzaSyAO_FJ2SlqU8Q4STEHLGCilw_Y9_11qcW8",
		userAgent:   "com.google.ios.youtube/19.45.4 (iPhone16,2; U; CPU iOS 18_1_0 like Mac OS X;)",
		deviceModel: "iPhone16,2",
	}
)

const (
	ContentPlaybackNonceAlphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-_"
)

func NewClient() *Client {
	return &Client{
		client: &IOSClient,
		Info: "iOSClient",
	}
}

func (c *Client) GetVideo(id string) (*Video, error) {
	ctx := context.Background()

	body, err := c.videoDataByInnertube(ctx, id)
	if err != nil {
		return nil, err
	}

	v := Video{
		ID: id,
	}

	if err = v.parseVideoInfo(body); err == nil {
		return &v, nil
	}

	return &v, nil
}

type innertubeRequest struct {
	VideoID         string            `json:"videoId,omitempty"`
	BrowseID        string            `json:"browseId,omitempty"`
	Continuation    string            `json:"continuation,omitempty"`
	Context         inntertubeContext `json:"context"`
	PlaybackContext *playbackContext  `json:"playbackContext,omitempty"`
	ContentCheckOK  bool              `json:"contentCheckOk,omitempty"`
	RacyCheckOk     bool              `json:"racyCheckOk,omitempty"`
	Params          string            `json:"params"`
}

type playbackContext struct {
	ContentPlaybackContext contentPlaybackContext `json:"contentPlaybackContext"`
}

type contentPlaybackContext struct {
	// SignatureTimestamp string `json:"signatureTimestamp"`
	HTML5Preference string `json:"html5Preference"`
}

type innertubeClient struct {
	HL                string `json:"hl"`
	GL                string `json:"gl"`
	ClientName        string `json:"clientName"`
	ClientVersion     string `json:"clientVersion"`
	AndroidSDKVersion int    `json:"androidSDKVersion,omitempty"`
	UserAgent         string `json:"userAgent,omitempty"`
	TimeZone          string `json:"timeZone"`
	UTCOffset         int    `json:"utcOffsetMinutes"`
	DeviceModel       string `json:"deviceModel,omitempty"`
	VisitorData       string `json:"visitorData,omitempty"`
}

type inntertubeContext struct {
	Client innertubeClient `json:"client"`
}

func (c *Client) videoDataByInnertube(ctx context.Context, id string) ([]byte, error) {
	data := innertubeRequest{
		VideoID:        id,
		Context:        prepareInnertubeContext(*c.client),
		ContentCheckOK: true,
		RacyCheckOk:    true,
		PlaybackContext: &playbackContext{
			ContentPlaybackContext: contentPlaybackContext{
				HTML5Preference: "HTML5_PREF_WANTS",
			},
		},
	}

	return c.httpPostBodyBytes(ctx, "https://www.youtube.com/youtubei/v1/player?key="+c.client.key, data)
}

var VisitorIdMaxAge = 10 * time.Hour

func (c *Client) getVisitorId() (string, error) {
	var err error
	if c.visitorId.value == "" || time.Since(c.visitorId.updated) > VisitorIdMaxAge {
		err = c.refreshVisitorId()
	}

	return c.visitorId.value, err
}

func (c *Client) refreshVisitorId() error {
	const sep = "\nytcfg.set("

	req, err := http.NewRequest(http.MethodGet, "https://www.youtube.com", nil)
	if err != nil {
		return err
	}

	resp, err := c.httpDo(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	_, data1, found := strings.Cut(string(data), sep)
	if !found {
		return err
	}
	var value struct {
		InnertubeContext struct {
			Client struct {
				VisitorData string
			}
		} `json:"INNERTUBE_CONTEXT"`
	}
	if err := json.NewDecoder(strings.NewReader(data1)).Decode(&value); err != nil {
		return err
	}

	if c.visitorId.value, err = url.PathUnescape(value.InnertubeContext.Client.VisitorData); err != nil {
		return err
	}

	c.visitorId.updated = time.Now()
	return nil
}

func (c *Client) httpPost(ctx context.Context, url string, body interface{}) (*http.Response, error) {
	data, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		return nil, err
	}

	req.Header.Set("X-Youtube-Client-Name", "3")
	req.Header.Set("X-Youtube-Client-Version", c.client.version)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")

	if xgoogvisitorid, err := c.getVisitorId(); err != nil {
		return nil, err
	} else {
		req.Header.Set("x-goog-visitor-id", xgoogvisitorid)
	}

	resp, err := c.httpDo(req)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, ErrUnexpectedStatusCode(resp.StatusCode)
	}

	return resp, nil
}

func (c *Client) httpDo(req *http.Request) (*http.Response, error) {
	client := c.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}

	req.Header.Set("User-Agent", c.client.userAgent)
	req.Header.Set("Origin", "https://youtube.com")
	req.Header.Set("Sec-Fetch-Mode", "navigate")

	if len(c.consentID) == 0 {
		c.consentID = strconv.Itoa(rand.Intn(899) + 100) //nolint:gosec
	}

	req.AddCookie(&http.Cookie{
		Name:   "CONSENT",
		Value:  "YES+cb.20210328-17-p0.en+FX+" + c.consentID,
		Path:   "/",
		Domain: ".youtube.com",
	})

	res, err := client.Do(req)

	log := slog.With("method", req.Method, "url", req.URL)

	if err == nil && res.StatusCode != http.StatusOK {
		err = ErrUnexpectedStatusCode(res.StatusCode)
		res.Body.Close()
		res = nil
	}

	if err != nil {
		log.Debug("HTTP request failed", "error", err)
	} else {
		log.Debug("HTTP request succeeded", "status", res.Status)
	}

	return res, err
}

func (c *Client) httpPostBodyBytes(ctx context.Context, url string, body interface{}) ([]byte, error) {
	resp, err := c.httpPost(ctx, url, body)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	return io.ReadAll(resp.Body)
}

func prepareInnertubeContext(clientInfo clientInfo) inntertubeContext {
	return inntertubeContext{
		Client: innertubeClient{
			HL:                "en",
			GL:                "US",
			TimeZone:          "UTC",
			DeviceModel:       clientInfo.deviceModel,
			ClientName:        clientInfo.name,
			ClientVersion:     clientInfo.version,
			AndroidSDKVersion: clientInfo.androidVersion,
			UserAgent:         clientInfo.userAgent,
			VisitorData:       randomVisitorData("US"),
		},
	}
}

func randomVisitorData(countryCode string) string {
	var pbE2 ProtoBuilder

	pbE2.String(2, "")
	pbE2.Varint(4, int64(rand.Intn(255)+1))

	var pbE ProtoBuilder
	pbE.String(1, countryCode)
	pbE.Bytes(2, pbE2.ToBytes())

	var pb ProtoBuilder
	pb.String(1, randString(ContentPlaybackNonceAlphabet, 11))
	pb.Varint(5, time.Now().Unix()-int64(rand.Intn(600000)))
	pb.Bytes(6, pbE.ToBytes())

	return pb.ToURLEncodedBase64()
}

func randString(alphabet string, sz int) string {
	var buf strings.Builder
	buf.Grow(sz)
	for i := 0; i < sz; i++ {
		buf.WriteByte(alphabet[rand.Intn(len(alphabet))])
	}
	return buf.String()
}

