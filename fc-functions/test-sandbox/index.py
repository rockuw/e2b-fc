# FC function for e2b-fc that proxies HTTP requests to envd
import json
import logging
import base64
import urllib.request
import urllib.error
import urllib.parse

logger = logging.getLogger()
logger.setLevel(logging.INFO)

# envd runs on this port inside the container
ENVD_PORT = 49983
ENVD_HOST = f"127.0.0.1:{ENVD_PORT}"


def handler(event, context):
    """
    FC function handler that proxies HTTP requests to envd.

    The e2b SDK sends Connect RPC requests to this function.
    We forward them to envd which handles process/filesystem operations.
    """
    logger.info(f"Received request, request_id: {context.request_id}")

    try:
        # Parse FC event (HTTP request mapped to event object)
        if isinstance(event, bytes):
            event_str = event.decode('utf-8')
        elif isinstance(event, str):
            event_str = event
        else:
            event_str = str(event)

        try:
            event_obj = json.loads(event_str) if event_str else {}
        except json.JSONDecodeError:
            event_obj = {}

        # Extract HTTP request details from FC event
        method = event_obj.get('requestContext', {}).get('http', {}).get('method', 'GET')
        raw_path = event_obj.get('rawPath', '/')
        headers = event_obj.get('headers', {})
        body_raw = event_obj.get('body', '')
        is_base64_encoded = event_obj.get('isBase64Encoded', False)

        # Decode body if base64 encoded
        if is_base64_encoded and body_raw:
            try:
                body = base64.b64decode(body_raw)
            except Exception:
                body = body_raw.encode('utf-8')
        else:
            body = body_raw.encode('utf-8') if isinstance(body_raw, str) else body_raw

        # Build the URL for envd
        # Remove any prefix that FC adds - forward the original path
        envd_url = f"http://{ENVD_HOST}{raw_path}"

        # Build urllib request headers - normalize header keys (FC capitalizes them)
        urllib_headers = {}
        for key, value in headers.items():
            # FC capitalizes header keys, convert to lowercase for urllib
            key_lower = key.lower()
            # Skip hop-by-hop headers
            if key_lower not in ('host', 'connection', 'transfer-encoding'):
                urllib_headers[key_lower] = value

        # Log the proxy request
        logger.info(f"Proxying {method} {raw_path} -> {envd_url}")

        # Create and execute the request
        req = urllib.request.Request(
            envd_url,
            data=body,
            headers=urllib_headers,
            method=method
        )

        # Execute request with timeout
        try:
            with urllib.request.urlopen(req, timeout=300) as response:
                resp_body = response.read()
                resp_status = response.status
                resp_headers = dict(response.headers)

                # Build FC response
                # For binary responses, base64 encode them
                content_type = resp_headers.get('Content-Type', 'application/octet-stream')
                is_resp_base64 = False

                # Check if content is binary
                if not content_type.startswith('text/') and not content_type.startswith('application/json'):
                    is_resp_base64 = True
                    resp_body = base64.b64encode(resp_body).decode('utf-8')

                fc_response = {
                    "statusCode": resp_status,
                    "headers": {
                        "Content-Type": content_type,
                        "X-Fc-Request-Id": context.request_id,
                    },
                    "body": resp_body,
                    "isBase64Encoded": is_resp_base64,
                }

                logger.info(f"Response status: {resp_status}, base64: {is_resp_base64}")
                return fc_response

        except urllib.error.HTTPError as e:
            # envd returned an error
            resp_body = e.read()
            fc_response = {
                "statusCode": e.code,
                "headers": {
                    "Content-Type": e.headers.get('Content-Type', 'application/json'),
                    "X-Fc-Request-Id": context.request_id,
                },
                "body": resp_body.decode('utf-8', errors='replace') if resp_body else e.reason,
                "isBase64Encoded": False,
            }
            logger.warning(f"HTTP error from envd: {e.code} {e.reason}")
            return fc_response

        except urllib.error.URLError as e:
            # Could not connect to envd
            logger.error(f"Failed to connect to envd at {ENVD_HOST}: {e.reason}")
            fc_response = {
                "statusCode": 502,
                "headers": {
                    "Content-Type": "application/json",
                    "X-Fc-Request-Id": context.request_id,
                },
                "body": json.dumps({
                    "error": "Bad Gateway",
                    "message": f"envd is not available at {ENVD_HOST}: {e.reason}"
                }),
                "isBase64Encoded": False,
            }
            return fc_response

    except Exception as e:
        logger.error(f"Error handling request: {e}")
        fc_response = {
            "statusCode": 500,
            "headers": {
                "Content-Type": "application/json",
                "X-Fc-Request-Id": context.request_id if hasattr(context, 'request_id') else 'unknown',
            },
            "body": json.dumps({
                "error": "Internal Server Error",
                "message": str(e)
            }),
            "isBase64Encoded": False,
        }
        return fc_response
