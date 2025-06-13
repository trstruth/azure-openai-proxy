# Azure OpenAI Managed-Identity Proxy

Tiny reverse-proxy that lets any client which **only supports the classic `api-key` header** talk to an Azure OpenAI deployment that is protected by Microsoft Entra ID (Managed Identity).

The proxy listens on a local TCP port, exchanges the presented (optional) shared secret for a **Managed-Identity token** and forwards the request to your Azure OpenAI instance while streaming the response back to the caller.  
It is completely stateless apart from a short in-memory token cache.

---

## 1 – Prerequisites

* Go 1.22+ installed (only required to build/run the proxy; the caller can be any language).
* The **machine / container / app-service** that will run the proxy must have a **Managed Identity** which is **assigned the _Cognitive Services User_ (or higher) role** on the target Azure OpenAI resource.


## 2 – Configure the target endpoint

`TARGET_URL` must point at the base URL **including the `/openai/` segment** of your resource, e.g.

```
export TARGET_URL="https://my-aoai.eastus2.inference.azure.com/openai/"
```

You can keep this in your shell, a `.env`, or edit the provided `Makefile`:

```make
# Makefile
run:
	TARGET_URL="https://my-aoai.openai.azure.com/openai/" go run .
```

Other environment variables (all optional):

| Variable            | Default                                            | Purpose                                                     |
|---------------------|----------------------------------------------------|-------------------------------------------------------------|
| `PORT`              | `8081`                                             | Local port to listen on                                      |
| `EXPECTED_KEY`      | _empty_                                            | If set, clients must pass this value via `api-key` header    |
| `AZURE_OPENAI_SCOPE`| `https://cognitiveservices.azure.com/.default`      | Scope used when requesting the AAD token                     |


## 3 – Build & run

```bash
# build binary
go build -o aoai-proxy .

# or run directly (shown here with Makefile)
make run   # TARGET_URL must be defined as shown above
```

Logs look like:

```
proxy listening on :8081 ➜ https://my-aoai.openai.azure.com/openai/
POST /deployments/gpt-4o/chat/completions?api-version=2024-02-15 from 127.0.0.1:57852
```


## 4 – Point your client at the proxy

Update your client / provider configuration to hit `http://localhost:8081` (or whichever host:port you used).

For example with [codex](https://github.com/openai/openai-codex-cli):

> NOTE: ensure you define the `AZURE_API_KEY` env var wherever you are running your client. You can set it equal to a garbage value like `foo`. codex will refuse to start if this key is not defined, but it is ignored as soon as your request hits the proxy. What matters is the _presence_ of the environment variable, not the value.

```bash
codex --provider azure \ 
      --base-url http://localhost:8081 \  # ← the proxy
      --deployment gpt-4o
```

If you configured `EXPECTED_KEY`, add it via:

```bash
export OPENAI_API_KEY=my-shared-secret
```


---

## Debugging tips

• Set `AZURE_LOG_LEVEL=debug` to see SDK details about Managed-Identity token acquisition.  
• `curl -v http://localhost:8081/` will show whether the proxy is reachable.  
• `curl -v -H "api-key: <secret>" ...` is useful if you enabled `EXPECTED_KEY`.  
• 401/403 from the upstream usually mean the Managed Identity has not been granted access to the Azure OpenAI resource.  
• Remember that `TARGET_URL` **must include a trailing slash and the `/openai/` segment** – otherwise the final URL will be malformed.


---

## How it works (brief)

1. Client sends the normal OpenAI-style request to the proxy.  
2. Proxy obtains / reuses an AAD token for the scope `https://cognitiveservices.azure.com/.default`.  
3. It strips any client `api-key` header, adds `Authorization: Bearer <token>`, and forwards to `TARGET_URL` while preserving the rest of the path & query string.  
4. The upstream response (including Server-Sent-Events) is streamed back untouched.


---

### Caveats

* **No TLS termination** – run it behind a reverse-proxy or use `localhost` if you need HTTPS.  
* Not suitable for multi-tenant scenarios unless you put it behind additional auth.  
* Only the subset of headers relevant to OpenAI requests are forwarded; hop-by-hop headers are dropped intentionally.
