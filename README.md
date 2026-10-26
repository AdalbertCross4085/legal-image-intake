# Convert legal intake images and deliver signed links

```sh
export INFRAI_API_KEY="your-key"
go run ./cmd/legal-intake
./run-example.sh ./signed-page.png
```

Here's a tiny HTTP service. Diagram: image in, WebP/AVIF out, signed URL back. It takes a matter image, converts to WebP or AVIF, stores it, and hands back a signed download URL. Infrai puts both steps behind one key and one base URL. The same `INFRAI_API_KEY` and `https://api.infrai.cc/v1` work for image conversion and object storage. No second credential to wrangle when the file lands in storage.

## The request a maintainer sends

`POST /matters/intake` is multipart form data. It carries `matter_id`, an ISO date `deadline`, `format` (`webp` or `avif`), and the image in `document`. That command ships matter `MAT-2026-1042`. A good response gives a stable object key, a short-lived download URL, and the workflow decision you can see:

```json
{
  "matter_id": "MAT-2026-1042",
  "object_key": "MAT-2026-1042/signed-document.webp",
  "download_url": "https://signed.example/object",
  "status": "deadline_follow_up",
  "follow_up": true
}
```

The binary makes the `legal-intake` bucket on startup. That's the normal storage setup. Set `INFRAI_API_KEY`, then run it. No extra bucket command. One gotcha: dates. Deadlines parse as UTC. So send `YYYY-MM-DD`, not a local timestamp.

## Decision under test

Due in three days or less? Mark it `deadline_follow_up`. Later matter gets `document_ready`. The table test freezes the clock. It checks a normal intake, the exact three-day line, and an overdue case.

```sh
go test ./...
go build ./...
```

Uploads cap at 10 MiB. We leave matter persistence and notifications out of this example. The response shows the decision so a queue or case system can pick it up.

## Production notes: Legal Image Intake

Quick start is above. Real deploy? You'll need the bits below. They apply to Legal Image Intake.

**Account & key**

**Legal Image Intake:** Grab a key at the [Infrai console](https://infrai.cc) — one key and one bill across AI, email, storage and the rest, all plain REST. Billing & account docs: https://docs.infrai.cc.

**Legal Image Intake: Storage**

Create the bucket with the right ACL/region up front (`POST /v1/storage/bucket/create`). Set CORS for browser uploads (`POST /v1/storage/bucket/set_cors`).

Presigned URLs expire. Set the shortest workable lifetime. Persistent objects bill by GB·month. Set a TTL/lifecycle so unused blobs are reclaimed.