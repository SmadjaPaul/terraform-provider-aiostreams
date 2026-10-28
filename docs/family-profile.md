# Family FR/EN + VOSTFR configuration contract

Source of truth candidate: [homelab-infra](https://github.com/SmadjaPaul/homelab-infra/blob/feat/aiostreams-provider-admission/docs/architecture/aiostreams-family-fr-en.yaml). This is a desired-state **contract**, not native UserData JSON and not yet applied.

## Native mechanisms to reuse

- AIOStreams configuration templates (`metadata` + partial `config`) can be provided by `TEMPLATE_URLS` or /app/data/templates; use these for bootstrapping and human-visible preset distribution.
- User API `POST /api/v1/user` creates saved configuration and `PUT` updates full config; provider owns one whole document.
- Provider must never mix template partial-merge semantics with whole-config PUT without schema validation and lossless preservation of unknown fields.
- Addons are installed by native manifest/preset configuration; configuration manifests may contain bearer tokens and cannot be committed.

## Functional goals

Films and series: FR preferred, EN fallback, FR subtitles available and selectable; no false claim that all titles offer these tracks. Anime: original JA audio with French subtitles (ASS/SSA retained where client supports them). Metadata: FR descriptions, artwork, season/episode navigation. Subtitles: SubSense plus optional supported fallback. Catalog fallback: Cinemeta; enrich with configured AIOMetadata or TMDB. TorBox key: Doppler `cluster/prd`, `APP_TORBOX_API_KEY`.

Optional stream discovery candidates Comet, StremThru Torz, Meteor, Lumio require manifest/availability/legal-content and TorBox compatibility qualification before enabling. Anime candidates SeaDex, AnimeTosho, NekoBT are discovery sources, not proof of playable legally authorized media.

## Required validation before Atlantis

Unit: no secrets in plans/state, correct lifecycle, API error suppression, idempotence, conflicting update protection, deletion gate.
Contract: native UserData schema round-trip, manifest `catalog`, `meta`, `stream` and `subtitles` compatibility.
Runtime: licensed film, episode and anime audio/subtitle playback; per-persona history isolation; direct CDN network proof.
For local smoke, only disposable AIOStreams config and local state. Production state location/identity must be created and tested before Atlantis apply.
