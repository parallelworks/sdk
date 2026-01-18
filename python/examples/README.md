# Python Client Examples

## Prerequisites

- Python 3.9 or later
- [uv](https://docs.astral.sh/uv/) (recommended) or pip
- A Parallel Works API key or token (from your ACTIVATE account settings)

## Running the Examples

### With uv (recommended)

```bash
# Set your API key or token
export PW_API_KEY="your-api-key-or-token"

# Run the sync example
uv run list_resources.py

# Run the async example
uv run list_resources_async.py
```

### With pip

```bash
# Create and activate a virtual environment
python -m venv .venv
source .venv/bin/activate

# Install the client from the parent directory
pip install ..

# Set your API key or token
export PW_API_KEY="your-api-key-or-token"

# Run the example
python list_resources.py
```

The platform host is automatically extracted from your credential.

## Expected Output

```
Fetching resources...

Buckets (2):
  - my-data-bucket (AWS)
  - archive-bucket (GCP)

Clusters (1):
  - dev-cluster (on)
```
