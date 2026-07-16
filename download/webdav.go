// Package download fetches source files from the Federal Revenue (via WebDAV)
// and the National Treasure (via CKAN).
package download

import (
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"path"
	"strings"
)

const (
	webdavURL      = "https://arquivos.receitafederal.gov.br/public.php/webdav/"
	cnpjToken      = "YggdBLfdninEJX9"
	taxRegimeToken = "MPPfFit7g7zdA8C"
	userAgent      = "Minha Receita/0.1.0 (minhareceita.org)"
)

type collectionMarker struct{}

// entry is a single WebDAV resource (file or directory) returned by PROPFIND.
type entry struct {
	Href          string            `xml:"href"`
	DisplayName   string            `xml:"propstat>prop>displayname"`
	ContentLength int64             `xml:"propstat>prop>getcontentlength"`
	LastModified  string            `xml:"propstat>prop>getlastmodified"`
	Collection    *collectionMarker `xml:"propstat>prop>resourcetype>collection"`
}

type multistatus struct {
	Responses []entry `xml:"response"`
}

// webdav is a client for a Nextcloud public share.
type webdav struct {
	client *http.Client
	base   string
	token  string
}

func newClient(token string) *webdav {
	return &webdav{
		client: &http.Client{},
		base:   webdavURL,
		token:  token,
	}
}

func (c *webdav) req(method, path string, body io.Reader) (*http.Request, error) {
	u := c.base + path
	r, err := http.NewRequest(method, u, body)
	if err != nil {
		return nil, fmt.Errorf("error creating %s request for %s: %w", method, u, err)
	}
	r.SetBasicAuth(c.token, "")
	r.Header.Set("User-Agent", userAgent)
	return r, nil
}

func (c *webdav) list(dir string) ([]entry, error) {
	body := `<?xml version="1.0"?><d:propfind xmlns:d="DAV:"><d:prop><d:displayname/><d:getcontentlength/><d:getlastmodified/><d:resourcetype/></d:prop></d:propfind>`
	r, err := c.req("PROPFIND", dir+"/", strings.NewReader(body))
	if err != nil {
		return nil, err
	}
	r.Header.Set("Content-Type", "text/xml")
	r.Header.Set("Depth", "1")
	resp, err := c.client.Do(r)
	if err != nil {
		return nil, fmt.Errorf("error listing %s: %w", dir, err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			return
		}
	}()
	if resp.StatusCode != http.StatusMultiStatus {
		return nil, fmt.Errorf("listing %s returned %s", dir, resp.Status)
	}
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("error reading listing response for %s: %w", dir, err)
	}
	var ms multistatus
	if err := xml.Unmarshal(b, &ms); err != nil {
		return nil, fmt.Errorf("error parsing listing response for %s: %w", dir, err)
	}
	return ms.Responses, nil
}

// listFilesByName is a fallback for servers that block WebDAV PROPFIND
// requests but still allow HEAD and GET requests for individual files.
func (c *webdav) listFilesByName(dir string, names []string) ([]entry, error) {
	entries := make([]entry, 0, len(names))
	for _, name := range names {
		filePath := path.Join(dir, name)
		r, err := c.req(http.MethodHead, filePath, nil)
		if err != nil {
			return nil, err
		}
		resp, err := c.client.Do(r)
		if err != nil {
			return nil, fmt.Errorf("error checking %s: %w", filePath, err)
		}
		if err := resp.Body.Close(); err != nil {
			return nil, fmt.Errorf("error closing response for %s: %w", filePath, err)
		}
		switch resp.StatusCode {
		case http.StatusOK:
			entries = append(entries, entry{
				DisplayName:   name,
				ContentLength: resp.ContentLength,
				LastModified:  resp.Header.Get("Last-Modified"),
			})
		case http.StatusNotFound:
			continue
		default:
			return nil, fmt.Errorf("checking %s returned %s", filePath, resp.Status)
		}
	}
	return entries, nil
}

func (c *webdav) download(path string, w io.Writer) (int64, error) {
	return c.downloadFrom(path, 0, w)
}

func (c *webdav) downloadFrom(path string, offset int64, w io.Writer) (int64, error) {
	r, err := c.req("GET", path, nil)
	if err != nil {
		return 0, err
	}
	if offset > 0 {
		r.Header.Set("Range", fmt.Sprintf("bytes=%d-", offset))
	}
	resp, err := c.client.Do(r)
	if err != nil {
		return 0, fmt.Errorf("error downloading %s: %w", path, err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			return
		}
	}()
	expectedStatus := http.StatusOK
	if offset > 0 {
		expectedStatus = http.StatusPartialContent
	}
	if resp.StatusCode != expectedStatus {
		return 0, fmt.Errorf("download of %s returned %s", path, resp.Status)
	}
	n, err := io.Copy(w, resp.Body)
	if err != nil {
		return n, fmt.Errorf("error saving %s: %w", path, err)
	}
	return n, nil
}
