package askuser

import (
	"context"
	"encoding/json"
	"errors"
	"html/template"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	logSDK "github.com/Laisky/go-utils/v5/log"
	"github.com/Laisky/zap"
	"github.com/google/uuid"
)

// NewHTTPHandler builds an HTTP multiplexer exposing the ask_user dashboard and APIs.
func NewHTTPHandler(service *Service, logger logSDK.Logger) http.Handler {
	handler := &httpHandler{
		service: service,
		logger:  logger,
	}

	if handler.logger == nil {
		handler.logger = serviceLogger()
	}

	page, err := template.New("askuser").Parse(pageHTML)
	if err != nil {
		handler.log().Warn("parse ask_user fallback page", zap.Error(err))
	} else {
		handler.page = page
	}

	handler.bootstrapStaticAssets()

	mux := http.NewServeMux()
	mux.HandleFunc("/api/requests", handler.handleRequests)
	mux.HandleFunc("/api/requests/", handler.handleRequestByID)

	if handler.static != nil {
		assetServer := http.StripPrefix("/", handler.static)
		mux.Handle("/assets/", assetServer)
		mux.Handle("/favicon.ico", assetServer)
		mux.Handle("/manifest.webmanifest", assetServer)
		mux.Handle("/robots.txt", assetServer)
	}

	mux.HandleFunc("/", handler.servePage)
	return mux
}

type httpHandler struct {
	service *Service
	logger  logSDK.Logger
	page    *template.Template
	index   []byte
	static  http.Handler
}

func (h *httpHandler) log() logSDK.Logger {
	if h.logger != nil {
		return h.logger
	}
	return serviceLogger()
}

func (h *httpHandler) servePage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	if len(h.index) > 0 {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Pragma", "no-cache")
		if r.Method == http.MethodGet {
			if _, err := w.Write(h.index); err != nil {
				h.log().Warn("write ask_user index", zap.Error(err))
			}
		}
		return
	}

	if h.page == nil {
		http.Error(w, "ask_user console not available", http.StatusServiceUnavailable)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if r.Method == http.MethodHead {
		return
	}
	if err := h.page.Execute(w, nil); err != nil {
		h.log().Warn("render ask_user page", zap.Error(err))
	}
}

func (h *httpHandler) handleRequests(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		h.listRequests(w, r)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (h *httpHandler) handleRequestByID(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/requests/")
	if path == "" {
		http.NotFound(w, r)
		return
	}

	id, err := uuid.Parse(path)
	if err != nil {
		http.Error(w, "invalid request id", http.StatusBadRequest)
		return
	}

	switch r.Method {
	case http.MethodPost:
		h.answerRequest(w, r, id)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (h *httpHandler) listRequests(w http.ResponseWriter, r *http.Request) {
	auth, err := ParseAuthorizationContext(r.Header.Get("Authorization"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}
	if h.service == nil {
		http.Error(w, "ask_user service unavailable", http.StatusServiceUnavailable)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	pending, history, err := h.service.ListRequests(ctx, auth)
	if err != nil {
		h.log().Error("list ask_user requests", zap.Error(err))
		http.Error(w, "failed to load requests", http.StatusInternalServerError)
		return
	}

	writeJSON(w, map[string]any{
		"pending":  serializeRequests(pending),
		"history":  serializeRequests(history),
		"user_id":  auth.UserIdentity,
		"ai_id":    auth.AIIdentity,
		"key_hint": auth.KeySuffix,
	})
}

func (h *httpHandler) answerRequest(w http.ResponseWriter, r *http.Request, id uuid.UUID) {
	if h.service == nil {
		http.Error(w, "ask_user service unavailable", http.StatusServiceUnavailable)
		return
	}
	auth, err := ParseAuthorizationContext(r.Header.Get("Authorization"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}

	payload := struct {
		Answer string `json:"answer"`
	}{}
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&payload); err != nil {
		http.Error(w, "invalid payload", http.StatusBadRequest)
		return
	}
	payload.Answer = strings.TrimSpace(payload.Answer)
	if payload.Answer == "" {
		http.Error(w, "answer cannot be empty", http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	req, err := h.service.AnswerRequest(ctx, auth, id, payload.Answer)
	if err != nil {
		status := http.StatusInternalServerError
		switch {
		case errors.Is(err, ErrRequestNotFound):
			status = http.StatusNotFound
		case errors.Is(err, ErrForbidden), errors.Is(err, ErrInvalidAuthorization):
			status = http.StatusForbidden
		}
		h.log().Warn("answer ask_user request", zap.Error(err))
		http.Error(w, err.Error(), status)
		return
	}

	writeJSON(w, map[string]any{
		"request": serializeRequest(*req),
	})
}

func serializeRequests(reqs []Request) []map[string]any {
	items := make([]map[string]any, 0, len(reqs))
	for _, req := range reqs {
		items = append(items, serializeRequest(req))
	}
	return items
}

func serializeRequest(req Request) map[string]any {
	result := map[string]any{
		"id":            req.ID.String(),
		"question":      req.Question,
		"status":        req.Status,
		"created_at":    req.CreatedAt,
		"updated_at":    req.UpdatedAt,
		"ai_identity":   req.AIIdentity,
		"user_identity": req.UserIdentity,
	}
	if req.Answer != nil {
		result["answer"] = *req.Answer
	}
	if req.AnsweredAt != nil {
		result["answered_at"] = req.AnsweredAt
	}
	return result
}

func writeJSON(w http.ResponseWriter, payload any) {
	w.Header().Set("Content-Type", "application/json")
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	_ = enc.Encode(payload)
}

func serviceLogger() logSDK.Logger {
	return logSDK.Shared.Named("ask_user_http")
}

func (h *httpHandler) bootstrapStaticAssets() {
	const envKey = "MCP_ASKUSER_DIST_DIR"

	var candidates []string
	if override := strings.TrimSpace(os.Getenv(envKey)); override != "" {
		candidates = append(candidates, override)
	}

	if exePath, err := os.Executable(); err == nil {
		exeDir := filepath.Dir(exePath)
		candidates = append(candidates,
			filepath.Join(exeDir, "dist"),
			filepath.Join(exeDir, "web", "dist"),
		)
	}

	if wd, err := os.Getwd(); err == nil {
		candidates = append(candidates,
			filepath.Join(wd, "dist"),
			filepath.Join(wd, "web", "dist"),
		)
	}

	if _, file, _, ok := runtime.Caller(0); ok {
		sourceDir := filepath.Dir(file)
		candidates = append(candidates,
			filepath.Join(sourceDir, "../../../../web/dist"),
		)
	}

	var distDir string
	for _, candidate := range candidates {
		if candidate == "" {
			continue
		}
		info, err := os.Stat(candidate)
		if err != nil || !info.IsDir() {
			continue
		}
		distDir = candidate
		break
	}

	if distDir == "" {
		h.log().Info("ask_user web assets not found, using fallback template")
		return
	}

	indexPath := filepath.Join(distDir, "index.html")
	indexBytes, err := os.ReadFile(indexPath)
	if err != nil {
		h.log().Warn("read ask_user index", zap.Error(err), zap.String("path", indexPath))
	} else {
		h.index = indexBytes
	}

	h.static = http.FileServer(http.Dir(distDir))
	h.log().Info("ask_user web assets mounted", zap.String("dist", distDir))
}

const pageHTML = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1" />
<title>ask_user Console</title>
<style>
    :root { color-scheme: light dark; }
    body { font-family: system-ui, sans-serif; margin: 0; padding: 0; background: #0f172a; color: #e2e8f0; }
    .wrapper { max-width: 960px; margin: 0 auto; padding: 32px 24px 48px; }
    h1 { font-size: 1.8rem; margin-bottom: 8px; }
    h2 { font-size: 1.3rem; margin-top: 32px; }
    p.lead { color: #94a3b8; margin-top: 0; }
	form#auth-form { display: flex; gap: 12px; flex-wrap: wrap; align-items: center; margin: 24px 0; }
	form#auth-form .input-wrapper { position: relative; flex: 1; min-width: 240px; }
	form#auth-form .input-wrapper input { width: 100%; padding: 10px 14px; padding-right: 96px; border-radius: 8px; border: none; background: rgba(15, 23, 42, 0.6); color: inherit; box-shadow: inset 0 0 0 1px rgba(148, 163, 184, 0.4); }
	form#auth-form .input-wrapper button { position: absolute; right: 10px; top: 50%; transform: translateY(-50%); padding: 6px 12px; border-radius: 6px; border: 1px solid rgba(148, 163, 184, 0.5); background: rgba(15, 23, 42, 0.4); color: #e2e8f0; font-size: 0.75rem; font-weight: 600; cursor: pointer; }
	form#auth-form .input-wrapper button:hover { background: rgba(148, 163, 184, 0.2); }
	form#auth-form > button { padding: 10px 18px; border-radius: 8px; border: none; background: #38bdf8; color: #0f172a; font-weight: 600; cursor: pointer; }
	form#auth-form > button:hover { background: #0ea5e9; }
    .card { background: rgba(15, 23, 42, 0.85); border-radius: 12px; padding: 18px 20px; margin-top: 16px; box-shadow: 0 12px 24px rgba(15, 23, 42, 0.35); }
    .card.pending { border: 1px solid rgba(56, 189, 248, 0.4); }
    .card.answered { border: 1px solid rgba(74, 222, 128, 0.3); }
    .meta { font-size: 0.85rem; color: #94a3b8; margin-bottom: 12px; display: flex; gap: 12px; flex-wrap: wrap; }
    .question { font-size: 1rem; margin-bottom: 12px; white-space: pre-wrap; }
    .answer { font-size: 0.95rem; margin-top: 12px; white-space: pre-wrap; background: rgba(148, 163, 184, 0.12); padding: 12px; border-radius: 8px; }
    .pending .answer-editor textarea { width: 100%; min-height: 96px; padding: 10px; border-radius: 8px; border: none; background: rgba(15, 23, 42, 0.6); color: inherit; box-shadow: inset 0 0 0 1px rgba(148, 163, 184, 0.4); resize: vertical; }
    .pending .answer-editor button { margin-top: 10px; padding: 8px 14px; border-radius: 6px; border: none; background: #22c55e; color: #0f172a; font-weight: 600; cursor: pointer; }
    .pending .answer-editor button:hover { background: #16a34a; }
    #status { margin-top: 12px; font-size: 0.9rem; }
    .hidden { display: none; }
    .history-empty { color: #64748b; font-style: italic; }
</style>
</head>
<body>
<div class="wrapper">
    <h1>ask_user Console</h1>
    <p class="lead">Review pending questions from your AI assistants and respond directly.</p>
	<form id="auth-form">
		<div class="input-wrapper">
			<input type="password" id="api-key" placeholder="Enter your API key" autocomplete="off" required />
			<button type="button" id="api-key-toggle" aria-pressed="false">Show</button>
		</div>
		<button type="submit">Connect</button>
	</form>
    <div id="status" class="hidden"></div>
    <section>
        <h2>Pending Questions</h2>
        <div id="pending-list"></div>
    </section>
    <section>
        <h2>History</h2>
        <div id="history-list"></div>
    </section>
</div>
<script>
(function() {
    const statusEl = document.getElementById('status');
    const pendingList = document.getElementById('pending-list');
    const historyList = document.getElementById('history-list');
	const form = document.getElementById('auth-form');
	const apiKeyInput = document.getElementById('api-key');
	const toggleBtn = document.getElementById('api-key-toggle');
	const STORAGE_KEY = 'ask_user_api_key';
	const API_BASE_PATH = (function() {
		try {
			const path = window.location.pathname || '/';
			return path.endsWith('/') ? path : path + '/';
		} catch (err) {
			return '/';
		}
	})();
	function normalizeApiKey(value) {
		let output = (value || '').trim();
		if (!output) {
			return '';
		}
		const prefix = /^Bearer\s+/i;
		while (prefix.test(output)) {
			output = output.replace(prefix, '').trim();
		}
		return output;
	}
	function buildAuthorizationHeader(key) {
		const token = normalizeApiKey(key);
		return token ? 'Bearer ' + token : '';
	}
	let apiKey = normalizeApiKey(localStorage.getItem(STORAGE_KEY) || '');
	let pollTimer = null;
	let isKeyVisible = false;

	function setKeyVisibility(visible) {
		isKeyVisible = Boolean(visible);
		if (apiKeyInput) {
			apiKeyInput.type = isKeyVisible ? 'text' : 'password';
		}
		if (toggleBtn) {
			toggleBtn.textContent = isKeyVisible ? 'Hide' : 'Show';
			toggleBtn.setAttribute('aria-pressed', isKeyVisible ? 'true' : 'false');
		}
	}

	if (toggleBtn) {
		toggleBtn.addEventListener('click', function() {
			setKeyVisibility(!isKeyVisible);
		});
	}

	if (apiKeyInput) {
		apiKeyInput.value = apiKey;
	}
	setKeyVisibility(false);

    function setStatus(message, isError) {
        if (!message) {
            statusEl.classList.add('hidden');
            statusEl.textContent = '';
            return;
        }
        statusEl.classList.remove('hidden');
        statusEl.textContent = message;
        statusEl.style.color = isError ? '#f87171' : '#22c55e';
    }

	function applyApiKey(key, options) {
		const normalized = normalizeApiKey(key);
		if (!normalized) {
			apiKey = '';
			localStorage.removeItem(STORAGE_KEY);
			if (apiKeyInput) {
				apiKeyInput.value = '';
			}
			pendingList.innerHTML = '';
			historyList.innerHTML = '';
			setKeyVisibility(false);
			setStatus('Disconnected.', false);
			stopPolling();
			return false;
		}

		const sameKey = normalized === apiKey;
		apiKey = normalized;
		localStorage.setItem(STORAGE_KEY, apiKey);
		if (apiKeyInput) {
			apiKeyInput.value = apiKey;
		}
		setKeyVisibility(false);

		if (options && options.initial) {
			setStatus('Connected. Fetching requests...', false);
		} else if (sameKey) {
			setStatus('Using stored API key.', false);
		} else {
			setStatus('Connecting…', false);
		}

		schedulePoll(0);
		return true;
	}

    function stopPolling() {
        if (pollTimer) {
            clearTimeout(pollTimer);
            pollTimer = null;
        }
    }

    function schedulePoll(delay) {
        stopPolling();
        pollTimer = setTimeout(fetchRequests, delay);
    }

    async function fetchRequests() {
        if (!apiKey) {
            return;
        }
		const authorization = buildAuthorizationHeader(apiKey);
		if (!authorization) {
			setStatus('Please provide an API key.', true);
			stopPolling();
			return;
		}
        try {
		const response = await fetch(API_BASE_PATH + 'api/requests', {
				headers: { 'Authorization': authorization }
            });
            if (!response.ok) {
                throw new Error(await response.text() || 'Failed to fetch requests');
            }
            const data = await response.json();
            renderRequests(data.pending || [], data.history || []);
            const identity = (data.user_id || '') + ' / ' + (data.ai_id || '');
            setStatus('Linked identities: ' + identity, false);
            schedulePoll(5000);
        } catch (err) {
            console.error(err);
            setStatus(err.message || 'Failed to fetch requests', true);
            schedulePoll(8000);
        }
    }

    function renderRequests(pending, history) {
        if (!Array.isArray(pending) || pending.length === 0) {
            pendingList.innerHTML = '<p class="history-empty">No pending questions.</p>';
        } else {
            pendingList.innerHTML = pending.map(renderPendingCard).join('');
            Array.from(pendingList.querySelectorAll('form[data-request-id]')).forEach(form => {
                form.addEventListener('submit', onAnswerSubmit);
            });
        }
        if (!Array.isArray(history) || history.length === 0) {
            historyList.innerHTML = '<p class="history-empty">No history yet.</p>';
        } else {
            historyList.innerHTML = history.map(renderHistoryCard).join('');
        }
    }

    function renderPendingCard(req) {
        var html = '';
        html += '<div class="card pending">';
        html += '<div class="meta">';
        html += '<span>ID: ' + req.id + '</span>';
        html += '<span>Asked: ' + formatDate(req.created_at) + '</span>';
        html += '<span>AI: ' + req.ai_identity + '</span>';
        html += '</div>';
        html += '<div class="question">' + escapeHTML(req.question) + '</div>';
        html += '<form class="answer-editor" data-request-id="' + req.id + '">';
        html += '<textarea placeholder="Provide your answer..." required></textarea>';
        html += '<button type="submit">Send answer</button>';
        html += '</form>';
        html += '</div>';
        return html;
    }

    function renderHistoryCard(req) {
        var html = '';
        html += '<div class="card answered">';
        html += '<div class="meta">';
        html += '<span>ID: ' + req.id + '</span>';
        html += '<span>Asked: ' + formatDate(req.created_at) + '</span>';
        html += '<span>Answered: ' + formatDate(req.answered_at) + '</span>';
        html += '</div>';
        html += '<div class="question">' + escapeHTML(req.question) + '</div>';
        if (req.answer) {
            html += '<div class="answer">' + escapeHTML(req.answer) + '</div>';
        } else {
            html += '<div class="answer">No answer provided.</div>';
        }
        html += '</div>';
        return html;
    }

    async function onAnswerSubmit(event) {
        event.preventDefault();
        const formEl = event.currentTarget;
        const textarea = formEl.querySelector('textarea');
        const requestId = formEl.dataset.requestId;
        if (!textarea || !requestId) {
            return;
        }
        const answer = textarea.value.trim();
        if (!answer) {
            return;
        }
		const authorization = buildAuthorizationHeader(apiKey);
		if (!authorization) {
			setStatus('Connect with your API key before answering.', true);
			return;
		}
        formEl.querySelector('button')?.setAttribute('disabled', 'disabled');
        try {
			const response = await fetch(API_BASE_PATH + 'api/requests/' + requestId, {
                method: 'POST',
                headers: {
                    'Content-Type': 'application/json',
					'Authorization': authorization
                },
                body: JSON.stringify({ answer: answer })
            });
            if (!response.ok) {
                throw new Error(await response.text() || 'Failed to submit answer');
            }
            textarea.value = '';
            schedulePoll(0);
            setStatus('Answer submitted successfully.', false);
        } catch (err) {
            console.error(err);
            setStatus(err.message || 'Failed to submit answer', true);
        } finally {
            formEl.querySelector('button')?.removeAttribute('disabled');
        }
    }

    function formatDate(input) {
        if (!input) {
            return 'N/A';
        }
        const date = new Date(input);
        if (Number.isNaN(date.getTime())) {
            return String(input);
        }
        return date.toLocaleString();
    }

    function escapeHTML(value) {
        return String(value || '').replace(/[&<>"']/g, function (c) {
            return ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' })[c];
        });
    }

	form.addEventListener('submit', function(event) {
		event.preventDefault();
		const rawValue = apiKeyInput ? apiKeyInput.value : '';
		const normalized = normalizeApiKey(rawValue);
		if (!normalized) {
			if (apiKey) {
				applyApiKey('');
			} else {
				setStatus('Please provide an API key.', true);
			}
			return;
		}
		applyApiKey(rawValue);
	});

	if (apiKey) {
		applyApiKey(apiKey, { initial: true });
	} else {
		setStatus('Disconnected.', false);
	}
})();
</script>
</body>
</html>`
