# hcli Demo

GIF demo deferred to post-release. This file documents how to create one.

## Recording

1. Build hcli: `go build -o /tmp/hcli ./cmd/hcli`
2. Seed synthetic data:
   ```sh
   /tmp/hcli collect -d
   # send synthetic OTLP spans via a test script
   /tmp/hcli stop
   ```
3. Record with asciinema:
   ```sh
   asciinema rec -c "/tmp/hcli tui" demo.cast
   ```
4. Convert to GIF:
   ```sh
   agg demo.cast demo.gif
   ```
5. Add `demo.gif` to README.md.
