# ICON GRIB Downloader

A Go program for downloading and managing GRIB files from the DWD (German Weather Service) ICON-EU model.

## Features

- Download GRIB files from the DWD's ICON-EU model
- Automatically find and download the latest model run
- Specify particular model runs and parameters
- Concurrent downloading to speed up the process
- Automatic decompression of .bz2 files
- Retry mechanism for failed downloads
- Organized output folder structure

## Installation

### Download Prebuilt Binaries

Prebuilt binaries for Windows, macOS, and Linux are available in the [Releases](https://github.com/yourusername/icon-grib-downloader/releases) section.

### Build from Source

```bash
# Clone the repository
git clone https://github.com/yourusername/icon-grib-downloader.git
cd icon-grib-downloader

# Build the program
go build -o icon-downloader

# Test the installation
./icon-downloader -version
```

## Usage Examples

### Download the Latest Model Run

```bash
./icon-downloader -latest
```

### Download a Specific Model Run

```bash
./icon-downloader -run 00
```

### Download Specific Parameters

```bash
./icon-downloader -run 12 -params t_2m,clct,pmsl
```

### Advanced Options

```bash
./icon-downloader -latest -outdir /path/to/output -concurrent 10 -retries 3 -verbose
```

## Command Line Options

| Option | Description | Default |
|--------|-------------|---------|
| `-run HH` | Specific model run to download (hour format HH) | |
| `-latest` | Download the latest available model run | |
| `-params list` | Comma-separated list of parameters to download | All parameters |
| `-outdir path` | Directory to save files | Current directory |
| `-concurrent N` | Maximum number of concurrent downloads | 5 |
| `-retries N` | Maximum number of retry attempts | 5 |
| `-verbose` | Enable detailed progress messages | false |
| `-version` | Show version information | |

## Output Structure

The downloaded files are organized in the following structure:

```
outputdir/
├── 00/
│   ├── t_2m_icon-eu_europe_regular-lat-lon_single-level_2023030600_000.grib2
│   ├── clct_icon-eu_europe_regular-lat-lon_single-level_2023030600_000.grib2
│   └── ...
└── 12/
    ├── t_2m_icon-eu_europe_regular-lat-lon_single-level_2023030612_000.grib2
    ├── clct_icon-eu_europe_regular-lat-lon_single-level_2023030612_000.grib2
    └── ...
```

## License

[MIT License](LICENSE)