/**
 * Cloudflare Worker — Agentic LLM Gateway Remote Strategy Config
 *
 * Deploy this worker to serve the routing strategy consumed by the gateway's
 * RemoteManager. The gateway polls this endpoint at the interval configured in
 * config.yaml (remote_strategy.poll_interval).
 *
 * Fields:
 *   strategy        - "local" or "remote" (which transport tier to use)
 *   local_model     - model name forwarded to the local_vllm provider
 *   remote_provider - which cloud provider to use when strategy == "remote"
 *   remote_model    - legacy single remote model; still respected by the router
 *   provider_models - per-provider model overrides; empty strings are ignored
 *                     (providers keep their current default when a value is "")
 *   fallback_on_404 - true  → retry with compile-time DefaultModel on HTTP 404
 *                     false → surface the 404 error to the caller immediately
 *   updated_at      - GMT+8 timestamp injected at request time for observability
 */
export default {
    async fetch(request, env, ctx) {
        // 1. Generate current time in GMT+8 (Asia/Shanghai)
        const now = new Date();

        const formatter = new Intl.DateTimeFormat("sv-SE", {
            timeZone: "Asia/Shanghai",
            year: "numeric",
            month: "2-digit",
            day: "2-digit",
            hour: "2-digit",
            minute: "2-digit",
            second: "2-digit",
        });

        const parts = formatter.formatToParts(now);
        const p = (type) => parts.find((part) => part.type === type).value;
        const formattedDate = `${p("year")}-${p("month")}-${p("day")} ${p("hour")}:${p("minute")}:${p("second")}`;

        // 2. Routing strategy config
        const config = {
            // ── Core routing ───────────────────────────────────────────────────────
            strategy: "remote",          // "local" | "remote"
            local_model: "qwen3-14b-awq",
            remote_provider: "google",   // must match a key in config.yaml providers
            remote_model: "gemini-3.0-flash-preview", // legacy field; kept for compatibility

            // ── Per-provider model overrides ───────────────────────────────────────
            // Applied after each fetch. Empty string → no-op (provider default kept).
            provider_models: {
                openai: "gpt-5",
                anthropic: "claude-3-5-haiku-20241022",
                google: "gemini-3.0-flash-preview",
                local_vllm: "qwen3-14b-awq",
            },

            // ── 404 fallback behaviour ─────────────────────────────────────────────
            // true  → on HTTP 404, retry once with the provider's compile-time default
            // false → surface the 404 error directly to the caller (no silent recovery)
            fallback_on_404: true,

            // ── Observability ──────────────────────────────────────────────────────
            updated_at: formattedDate,
        };

        return new Response(JSON.stringify(config), {
            headers: {
                "Content-Type": "application/json",
                "Cache-Control": "no-cache",
            },
        });
    },
};
