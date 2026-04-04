# Simple FC function for testing session functionality
import json
import logging

logger = logging.getLogger()
logger.setLevel(logging.INFO)

def handler(event, context):
    """
    FC function handler for session testing.
    
    This is a minimal function that responds to HTTP requests.
    In production, this would forward to envd for process/filesystem operations.
    """
    logger.info(f"Received event: {event}")
    
    # Parse request
    try:
        if isinstance(event, bytes):
            event = event.decode('utf-8')
        if isinstance(event, str):
            event = json.loads(event)
    except Exception as e:
        logger.error(f"Failed to parse event: {e}")
    
    # Return response
    response = {
        "statusCode": 200,
        "headers": {
            "Content-Type": "application/json"
        },
        "body": json.dumps({
            "message": "Hello from FC Session!",
            "function": "test-sandbox",
            "requestId": context.request_id if hasattr(context, 'request_id') else 'unknown'
        })
    }
    
    return response
