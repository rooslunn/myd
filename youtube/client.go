package youtube

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync/atomic"
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
	MaxRoutines int
	ChunkSize int64
	playerCache playerCache
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
	Size1Kb  = 1024
	Size1Mb  = Size1Kb * 1024
	Size10Mb = Size1Mb * 10

	playerParams = "CgIQBg=="
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

func (c *Client) GetStream(video *Video, format *Format) (io.ReadCloser, int64, error) {
	return c.GetStreamContext(context.Background(), video, format)
}

// GetStreamContext returns the stream and the total size for a specific format with a context.
func (c *Client) GetStreamContext(ctx context.Context, video *Video, format *Format) (io.ReadCloser, int64, error) {
	url, err := c.GetStreamURL(video, format)
	if err != nil {
		return nil, 0, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, 0, err
	}

	r, w := io.Pipe()
	contentLength := format.ContentLength

	if contentLength == 0 {
		// some videos don't have length information
		contentLength = c.downloadOnce(req, w, format)
	} else {
		// we have length information, let's download by chunks!
		c.downloadChunked(ctx, req, w, format)
	}

	return r, contentLength, nil
}

func (c *Client) downloadOnce(req *http.Request, w *io.PipeWriter, _ *Format) int64 {
	resp, err := c.httpDo(req)
	if err != nil {
		w.CloseWithError(err) //nolint:errcheck
		return 0
	}

	go func() {
		defer resp.Body.Close()
		_, err := io.Copy(w, resp.Body)
		if err == nil {
			w.Close()
		} else {
			w.CloseWithError(err) //nolint:errcheck
		}
	}()

	contentLength := resp.Header.Get("Content-Length")
	length, _ := strconv.ParseInt(contentLength, 10, 64)

	return length
}

func (c *Client) getChunkSize() int64 {
	if c.ChunkSize > 0 {
		return c.ChunkSize
	}

	return Size10Mb
}

func (c *Client) getMaxRoutines(limit int) int {
	routines := 10

	if c.MaxRoutines > 0 {
		routines = c.MaxRoutines
	}

	if limit > 0 && routines > limit {
		routines = limit
	}

	return routines
}

func (c *Client) downloadChunked(ctx context.Context, req *http.Request, w *io.PipeWriter, format *Format) {
	chunks := getChunks(format.ContentLength, c.getChunkSize())
	maxRoutines := c.getMaxRoutines(len(chunks))

	cancelCtx, cancel := context.WithCancel(ctx)
	abort := func(err error) {
		w.CloseWithError(err)
		cancel()
	}

	currentChunk := atomic.Uint32{}
	for i := 0; i < maxRoutines; i++ {
		go func() {
			for {
				chunkIndex := int(currentChunk.Add(1)) - 1
				if chunkIndex >= len(chunks) {
					// no more chunks
					return
				}

				chunk := &chunks[chunkIndex]
				err := c.downloadChunk(req.Clone(cancelCtx), chunk)
				close(chunk.data)

				if err != nil {
					abort(err)
					return
				}
			}
		}()
	}

	go func() {
		// copy chunks into the PipeWriter
		for i := 0; i < len(chunks); i++ {
			select {
			case <-cancelCtx.Done():
				abort(context.Canceled)
				return
			case data := <-chunks[i].data:
				_, err := io.Copy(w, bytes.NewBuffer(data))
				if err != nil {
					abort(err)
				}
			}
		}

		// everything succeeded
		w.Close()
	}()
}

// GetStreamURL returns the url for a specific format
func (c *Client) GetStreamURL(video *Video, format *Format) (string, error) {
	return c.GetStreamURLContext(context.Background(), video, format)
}

// GetStreamURLContext returns the url for a specific format with a context
func (c *Client) GetStreamURLContext(ctx context.Context, video *Video, format *Format) (string, error) {
	if format == nil {
		return "", ErrNoFormat
	}

	if format.URL != "" {
		if c.client.androidVersion > 0 {
			return format.URL, nil
		}

		return c.unThrottle(ctx, video.ID, format.URL)
	}

	// TODO: check rest of this function, is it redundant?

	cipher := format.Cipher
	if cipher == "" {
		return "", ErrCipherNotFound
	}

	uri, err := c.decipherURL(ctx, video.ID, cipher)
	if err != nil {
		return "", err
	}

	return uri, err
}

func (c *Client) unThrottle(ctx context.Context, videoID string, urlString string) (string, error) {
	config, err := c.getPlayerConfig(ctx, videoID)
	if err != nil {
		return "", err
	}

	uri, err := url.Parse(urlString)
	if err != nil {
		return "", err
	}

	// for debugging
	if artifactsFolder != "" {
		writeArtifact("video-"+videoID+".url", []byte(uri.String()))
	}

	query, err := c.decryptNParam(config, uri.Query())
	if err != nil {
		return "", err
	}

	uri.RawQuery = query.Encode()
	return uri.String(), nil
}

type playerConfig []byte

var basejsPattern = regexp.MustCompile(`(/s/player/\w+/player_ias.vflset/\w+/base.js)`)

func (c *Client) getPlayerConfig(ctx context.Context, videoID string) (playerConfig, error) {
	embedURL := fmt.Sprintf("https://youtube.com/embed/%s?hl=en", videoID)
	embedBody, err := c.httpGetBodyBytes(ctx, embedURL)
	if err != nil {
		return nil, err
	}

	// example: /s/player/f676c671/player_ias.vflset/en_US/base.js
	playerPath := string(basejsPattern.Find(embedBody))
	if playerPath == "" {
		return nil, errors.New("unable to find basejs URL in playerConfig")
	}

	// for debugging
	var artifactName string
	if artifactsFolder != "" {
		parts := strings.SplitN(playerPath, "/", 5)
		artifactName = "player-" + parts[3] + ".js"
		linkName := filepath.Join(artifactsFolder, "video-"+videoID+".js")
		if err := os.Symlink(artifactName, linkName); err != nil {
			slog.Info("unable to create symlink %s: %v", linkName, err)
		}
	}

	config := c.playerCache.Get(playerPath)
	if config != nil {
		return config, nil
	}

	config, err = c.httpGetBodyBytes(ctx, "https://youtube.com"+playerPath)
	if err != nil {
		return nil, err
	}

	// for debugging
	if artifactName != "" {
		writeArtifact(artifactName, config)
	}

	c.playerCache.Set(playerPath, config)
	return config, nil
}

func (c *Client) httpGetBodyBytes(ctx context.Context, url string) ([]byte, error) {
	resp, err := c.httpGet(ctx, url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	return io.ReadAll(resp.Body)
}

func (c *Client) httpGet(ctx context.Context, url string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
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

func (c *Client) downloadChunk(req *http.Request, chunk *chunk) error {
	q := req.URL.Query()
	q.Set("range", fmt.Sprintf("%d-%d", chunk.start, chunk.end))
	req.URL.RawQuery = q.Encode()

	resp, err := c.httpDo(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return ErrUnexpectedStatusCode(resp.StatusCode)
	}

	expected := int(chunk.end-chunk.start) + 1
	data, err := io.ReadAll(resp.Body)
	n := len(data)

	if err != nil {
		return err
	}

	if n != expected {
		return fmt.Errorf("chunk at offset %d has invalid size: expected=%d actual=%d", chunk.start, expected, n)
	}

	chunk.data <- data

	return nil
}

func (c *Client) decipherURL(ctx context.Context, videoID string, cipher string) (string, error) {
	params, err := url.ParseQuery(cipher)
	if err != nil {
		return "", err
	}

	uri, err := url.Parse(params.Get("url"))
	if err != nil {
		return "", err
	}
	query := uri.Query()

	config, err := c.getPlayerConfig(ctx, videoID)
	if err != nil {
		return "", err
	}

	// decrypt s-parameter
	bs, err := config.decrypt([]byte(params.Get("s")))
	if err != nil {
		return "", err
	}
	query.Add(params.Get("sp"), string(bs))

	query, err = c.decryptNParam(config, query)
	if err != nil {
		return "", err
	}

	uri.RawQuery = query.Encode()

	return uri.String(), nil
}

func (c *Client) decryptNParam(config playerConfig, query url.Values) (url.Values, error) {
	// decrypt n-parameter
	nSig := query.Get("v")
	// log := Logger.With("n", nSig)

	if nSig != "" {
		nDecoded, err := config.decodeNsig(nSig)
		if err != nil {
			return nil, fmt.Errorf("unable to decode nSig: %w", err)
		}
		query.Set("v", nDecoded)
		// log = log.With("decoded", nDecoded)
	}

	// log.Debug("nParam")

	return query, nil
}
