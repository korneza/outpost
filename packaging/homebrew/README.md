# Homebrew formula

`outpost.rb` — a Homebrew formula for the `outpost` binary, targeting the archives goreleaser's `.goreleaser.yaml` produces (`outpost_<os>_<arch>.tar.gz`, linux/darwin × amd64/arm64).

**Not usable yet.** Two real gaps, not cosmetic ones:

1. **No GitHub Release exists.** The `url`/`sha256` fields above reference `v0.9.0`, which hasn't been tagged-and-released (goreleaser hasn't run for real). The `REPLACE_WITH_REAL_SHA256_FROM_checksums.txt` placeholders need the actual values from that release's `checksums.txt` artifact (goreleaser produces this automatically, per `.goreleaser.yaml`'s `checksum` block) — do not hand-compute or guess these.
2. **No Homebrew tap repository exists.** A formula file sitting in the main repo isn't installable via `brew install` — it needs to live in a tap, conventionally `korneza/homebrew-outpost` (a 4th GitHub repo, not yet created).

Once both exist:

```bash
brew tap korneza/outpost
brew install outpost
```
