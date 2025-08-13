import csv
import json
import os
from urllib.request import Request, urlopen
from urllib.error import HTTPError, URLError


MINIFLUX_URL = os.environ.get("MINIFLUX_URL", "https://miniflux.syscd.tech")
MINIFLUX_TOKEN = os.environ.get("MINIFLUX_TOKEN", "")
CSV_PATH = os.environ.get("CHANNELS_CSV_PATH", "hack/youtube/subscribed_counts.csv")
CATEGORY_ID = int(os.environ.get("MINIFLUX_CATEGORY_ID", "1"))


def build_feed_url(channel_id: str) -> str:
    return f"https://www.youtube.com/feeds/videos.xml?channel_id={channel_id}"


def subscribe_feed(feed_url: str) -> int:
    payload = {"feed_url": feed_url, "category_id": CATEGORY_ID}
    body = json.dumps(payload).encode("utf-8")
    req = Request(
        f"{MINIFLUX_URL}/v1/feeds",
        data=body,
        headers={
            "Content-Type": "application/json",
            "X-Auth-Token": MINIFLUX_TOKEN,
        },
        method="POST",
    )
    try:
        with urlopen(req) as resp:
            return resp.getcode()
    except HTTPError as e:
        return e.code
    except URLError:
        return 0


def main() -> None:
    with open(CSV_PATH, newline="", encoding="utf-8") as f:
        reader = csv.DictReader(f)
        for row in reader:
            channel_name = row.get("channel_name", "").strip()
            channel_id = row.get("channel_id", "").strip()
            if not channel_id:
                continue
            status = subscribe_feed(build_feed_url(channel_id))
            print(f"{status}\t{channel_name}\t{channel_id}")


if __name__ == "__main__":
    main()


