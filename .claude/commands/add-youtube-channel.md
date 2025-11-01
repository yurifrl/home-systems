---
description: Add a YouTube channel to echotube tier1 list from video URL
---

Add a YouTube channel to k8s/charts/echotube/files/tier1.json by extracting the channel ID from any YouTube URL (video, channel, etc).

Steps:
1. Extract the YouTube URL from the user's request
2. Use `uvx yt-dlp --print channel_id,channel --skip-download <URL>` to extract:
   - channel_id (line 1)
   - channel name (line 2)
3. Read k8s/charts/echotube/files/tier1.json
4. Check if channel_id already exists (avoid duplicates)
5. If not exists, append new entry with format:
   ```json
   {
       "channel_name": "<extracted name>",
       "channel_id": "<extracted id>"
   }
   ```
6. Maintain proper JSON formatting with consistent indentation
7. Write updated JSON back to file

Arguments: YouTube URL (video, channel, or any YouTube URL)

Example: /add-youtube-channel https://www.youtube.com/watch?v=U2budy3S3MA
