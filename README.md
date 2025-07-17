# Song Splitter

This tool splits a single audio or video file into multiple tracks based on a provided tracklist. It uses `ffmpeg` for processing and can be run easily via Docker.

## Prerequisites

- Docker
- Docker Compose

## Usage

The application is run using Docker Compose to simplify command execution and volume mounting. The `docker-compose.yml` file is configured to mount the current directory into the `/data` directory in the container. This means you should place your input media file and `tracklist.txt` in the same directory as the `docker-compose.yml` file. The processed tracks will be saved to an `output` directory that will be created in the same location.

### Building the Docker Image

First, build the Docker image:

```bash
docker-compose build
```

### Running the Splitter

To run the application, you use `docker-compose run`. You need to provide the command-line arguments to the `song-splitter` service.

**Command-line flags:**

- `--tracklist <path>`: Path to the tracklist file (e.g., `tracklist.txt`).
- `--input <path>`: Path to the input media file (e.g., `input.mp4`).
- `--audio`: Split into audio tracks (MP3).
- `--video`: Split into video tracks (MP4).

**Example Commands:**

- **To split a video file into multiple MP4 video tracks:**

  ```bash
  docker-compose run song-splitter --input my_set.mp4 --tracklist tracklist.txt --video
  ```

- **To split a video or audio file into multiple MP3 audio tracks:**

  ```bash
  docker-compose run song-splitter --input my_set.mp4 --tracklist tracklist.txt --audio
  ```

The output files will be placed in the `output/` directory on your host machine.

### `tracklist.txt` Format

The tracklist file has a specific format. The first line is the album/set title. Subsequent lines represent tracks with their start time, artist, title, and optional label.

**Example `tracklist.txt`:**

```
My Awesome DJ Set
[0:00:00] Artist 1 - Title 1 [Label 1]
[0:03:30] Artist 2 - Title 2
w/ Artist 2.1 - Title 2.1 [Label 2.1]
[0:07:15] Artist 3 - Title 3 [Label 3]
```

- The first line (`My Awesome DJ Set`) is used as the "album" metadata tag.
- `[HH:MM:SS]` or `[MM:SS]` is the start time of the track.
- `Artist - Title` is the main track information.
- `[Label]` is the record label (optional).
- Lines starting with `w/` denote an additional track mixed with the main track.
