package imguploader

import (
	"bytes"
	"context"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"

	"github.com/grafana/grafana/pkg/internal/util"
)

type WebdavUploader struct {
	url        string
	username   string
	password   string
	public_url string
}

var netTransport = &http.Transport{
	Proxy: http.ProxyFromEnvironment,
	Dial: (&net.Dialer{
		Timeout: 60 * time.Second,
	}).Dial,
	TLSHandshakeTimeout: 5 * time.Second,
}

var netClient = &http.Client{
	Timeout:   time.Second * 60,
	Transport: netTransport,
}

func (u *WebdavUploader) PublicURL(filename string) string {
	if strings.Contains(u.public_url, "${file}") {
		return strings.ReplaceAll(u.public_url, "${file}", filename)
	}

	publicURL, _ := url.Parse(u.public_url)
	publicURL.Path = path.Join(publicURL.Path, filename)
	return publicURL.String()
}

func (u *WebdavUploader) Upload(ctx context.Context, imgToUpload string) (string, error) {
	url, _ := url.Parse(u.url)
	filename, err := util.GetRandomString(20)
	if err != nil {
		return "", err
	}

	filename += pngExt
	url.Path = path.Join(url.Path, filename)

	// We can ignore the gosec G304 warning on this one because `imgToUpload` comes
	// from alert notifiers and is only used to upload images generated by alerting.
	// nolint:gosec
	imgData, err := ioutil.ReadFile(imgToUpload)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequest("PUT", url.String(), bytes.NewReader(imgData))
	if err != nil {
		return "", err
	}
	if ctx != nil {
		req = req.WithContext(ctx)
	}
	if u.username != "" {
		req.SetBasicAuth(u.username, u.password)
	}

	res, err := netClient.Do(req)
	if err != nil {
		return "", err
	}
	defer func() {
		if err := res.Body.Close(); err != nil {
			logger.Warn("Failed to close response body", "err", err)
		}
	}()

	if res.StatusCode != http.StatusCreated {
		body, err := ioutil.ReadAll(res.Body)
		if err != nil {
			return "", fmt.Errorf("failed to read response body: %w", err)
		}
		return "", fmt.Errorf("failed to upload image, statuscode: %d, body: %s", res.StatusCode, body)
	}

	if u.public_url != "" {
		return u.PublicURL(filename), nil
	}

	return url.String(), nil
}

func NewWebdavImageUploader(url, username, password, public_url string) (*WebdavUploader, error) {
	return &WebdavUploader{
		url:        url,
		username:   username,
		password:   password,
		public_url: public_url,
	}, nil
}
