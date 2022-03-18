package main

import (
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"time"

	"github.com/mwitkow/go-conntrack"

	"go.uber.org/zap"
)

func NewPathPreservingProxy(targetConfig TargetConfig, proxyConfig ProxyConfig) (*httputil.ReverseProxy, error) {
	targetURL, err := url.Parse(targetConfig.Connection.HTTP.URL)
	if err != nil {
		return nil, err
	}

	proxy := httputil.NewSingleHostReverseProxy(targetURL)
	proxy.Director = func(req *http.Request) {
		req.Host = targetURL.Host
		req.URL.Scheme = targetURL.Scheme
		req.URL.Host = targetURL.Host

		// this bit right here makes sure that all the rpc URLs with
		// /<apikey> work.
		req.URL.Path = targetURL.Path

		// Workaround to reserve request body in ReverseProxy.ErrorHandler
		// see more here: https://github.com/golang/go/issues/33726
		if req.Body != nil && req.ContentLength != 0 {
			var buf bytes.Buffer
			var bodyReader io.Reader

			// If the body is gzip-ed but the target doesn't support request compression
			// we decompress the body before sending
			//
			// Edge case: target 1 doesn't support request compression but target 2 does
			// 	In this case, since the body is already decompressed to serve the target 1,
			//  in a reroute event, target 2 will just receive the decompressed body instead
			//  of the original compressed one.
			//  We could fix this by either re-compress the body or keep a copy of the original (gzipped) body.
			if req.Header.Get("Content-Encoding") == "gzip" && !targetConfig.Connection.HTTP.Compression {
				zap.L().Debug("go to gzip")

				gzr, err := gzip.NewReader(req.Body)

				if err != nil {
					zap.L().Error("error while initiate gzip reader", zap.Error(err))
					// Failed to read gzip content, treat it as uncompressed data
					bodyReader = io.TeeReader(req.Body, &buf)
				} else {
					// Decompress the body
					data, err := ioutil.ReadAll(gzr)

					if err != nil {
						panic(err)
					}

					// Replace body content with uncompressed data
					// Remove the "Content-Encoding: gzip" because the body is decompressed already
					// and correct the ContentLength header
					bodyReader = bytes.NewReader(data)
					req.Header.Del("Content-Encoding")
					req.ContentLength = int64(len(data))
				}
			} else {
				zap.L().Debug("not go to gzip")
				bodyReader = io.TeeReader(req.Body, &buf)
			}
			req.Body = io.NopCloser(bodyReader)

			ctx := context.WithValue(req.Context(), "bodybuf", &buf)
			r2 := req.WithContext(ctx)
			*req = *r2
		}

		zap.L().Debug(fmt.Sprintf("forwarding request to: %s", req.URL))
	}

	conntrackDialer := conntrack.NewDialContextFunc(
		conntrack.DialWithName(targetConfig.Name),
		conntrack.DialWithTracing(),
		conntrack.DialWithDialer(&net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
		}),
	)

	proxy.Transport = &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		DialContext:           conntrackDialer,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          100,
		IdleConnTimeout:       30 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		ResponseHeaderTimeout: proxyConfig.UpstreamTimeout,
	}

	conntrack.PreRegisterDialerMetrics(targetConfig.Name)

	return proxy, nil
}