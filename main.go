// main.go
package main

import (
	"context"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync/atomic"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
)

var (
	// required: e.g. https://my-aoai.eastus2.inference.azure.com or https://my-aoai.openai.azure.com
	target = mustParseURL("TARGET_URL")
	// optional key the client must present
	expectKey = os.Getenv("EXPECTED_KEY")
	// Azure scope for OpenAI when using Entra ID / MI
	scope = getEnv("AZURE_OPENAI_SCOPE", "https://cognitiveservices.azure.com/.default")
	port  = getEnv("PORT", "8081")
)

// simple in-process cache so we don’t hit IMDS on every request
type cachedToken struct {
	val string
	exp time.Time
}

var tokenCache atomic.Value // cachedToken

func main() {
	cred, err := azidentity.NewDefaultAzureCredential(nil)
	if err != nil {
		log.Fatalf("identity: %v", err)
	}

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		log.Printf("%s %s from %s", r.Method, r.URL.RequestURI(), r.RemoteAddr)

		if expectKey != "" && r.Header.Get("api-key") != expectKey && r.Header.Get("x-api-key") != expectKey {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		// strip key headers
		r.Header.Del("api-key")
		r.Header.Del("x-api-key")

		// obtain (or reuse) MI token
		tok, err := getToken(r.Context(), cred)
		if err != nil {
			log.Println("token acquisition failed")
			http.Error(w, "token acquisition failed", http.StatusInternalServerError)
			return
		}

		// build upstream request
		upURL := *target // copy
		upURL.Path = singleJoiningSlash(upURL.Path, r.URL.Path)
		upURL.RawQuery = r.URL.RawQuery
		upReq, err := http.NewRequestWithContext(r.Context(), r.Method, upURL.String(), r.Body)
		if err != nil {
			log.Println("bad upstream request")
			http.Error(w, "bad upstream request", http.StatusInternalServerError)
			return
		}
		copyHeaders(upReq.Header, r.Header)
		upReq.Header.Set("Authorization", "Bearer "+tok)

		// forward
		resp, err := http.DefaultClient.Do(upReq)
		if err != nil {
			log.Println("upstream error")
			http.Error(w, "upstream error", http.StatusBadGateway)
			return
		}
		defer resp.Body.Close()

		copyHeaders(w.Header(), resp.Header)
		w.WriteHeader(resp.StatusCode)
		io.Copy(w, resp.Body) // streams SSE responses as well
	})

	log.Printf("proxy listening on :%s ➜ %s", port, target)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}

// ---------- helpers ----------

func getToken(ctx context.Context, cred *azidentity.DefaultAzureCredential) (string, error) {
	if t, ok := tokenCache.Load().(cachedToken); ok && time.Now().Before(t.exp.Add(-1*time.Minute)) {
		return t.val, nil
	}
	tk, err := cred.GetToken(ctx, policy.TokenRequestOptions{Scopes: []string{scope}})
	if err != nil {
		return "", err
	}
	tokenCache.Store(cachedToken{val: tk.Token, exp: tk.ExpiresOn})
	return tk.Token, nil
}

func copyHeaders(dst, src http.Header) {
	for k, vv := range src {
		for _, v := range vv {
			// hop-by-hop headers deliberately excluded
			if strings.EqualFold(k, "Host") || strings.EqualFold(k, "Authorization") {
				continue
			}
			dst.Add(k, v)
		}
	}
}

func mustParseURL(env string) *url.URL {
	s := os.Getenv(env)
	if s == "" {
		log.Fatalf("%s not set", env)
	}
	u, err := url.Parse(s)
	if err != nil {
		log.Fatalf("%s: %v", env, err)
	}
	return u
}

func getEnv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func singleJoiningSlash(a, b string) string {
	switch {
	case strings.HasSuffix(a, "/") && strings.HasPrefix(b, "/"):
		return a + b[1:]
	case strings.HasSuffix(a, "/") || strings.HasPrefix(b, "/"):
		return a + b
	default:
		return a + "/" + b
	}
}
