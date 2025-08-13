import json
import os
import sys
from urllib.request import Request, urlopen


MINIFLUX_URL = os.environ.get("MINIFLUX_URL", "https://miniflux.syscd.tech")
MINIFLUX_TOKEN = os.environ["MINIFLUX_TOKEN"]
CATEGORY_ID = int(os.environ.get("MINIFLUX_CATEGORY_ID", "1"))

def build_feed_url(channel_id: str) -> str:
    return f"https://www.youtube.com/feeds/videos.xml?channel_id={channel_id}"


def subscribe_feed(feed_url: str) -> int:
    body = json.dumps({"feed_url": feed_url, "category_id": CATEGORY_ID}).encode("utf-8")
    req = Request(
        f"{MINIFLUX_URL}/v1/feeds",
        data=body,
        headers={"Content-Type": "application/json", "X-Auth-Token": MINIFLUX_TOKEN},
        method="POST",
    )
    with urlopen(req) as resp:
        return resp.getcode()


def main() -> None:
    data = json.load(sys.stdin)

    if isinstance(data, dict):
        items = [{"channel_name": k, "channel_id": v} for k, v in data.items()]
    else:
        items = data

    for item in items:
        name = (item.get("channel_name") or "").strip()
        cid = (item.get("channel_id") or "").strip()
        if not cid:
            continue
        status = subscribe_feed(build_feed_url(cid))
        print(f"{status}\t{name}\t{cid}")


if __name__ == "__main__":
    main()


