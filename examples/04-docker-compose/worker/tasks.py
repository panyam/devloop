import os
import time
import logging
from celery import Celery

# Configure logging
logging.basicConfig(level=logging.INFO)
logger = logging.getLogger(__name__)

# Initialize Celery
broker_url = os.getenv('CELERY_BROKER_URL', 'redis://redis:6379/0')
result_backend = os.getenv('CELERY_RESULT_BACKEND', 'redis://redis:6379/0')

app = Celery('docker_compose_worker', broker=broker_url, backend=result_backend)

# Configure Celery
app.conf.update(
    task_serializer='json',
    accept_content=['json'],
    result_serializer='json',
    timezone='UTC',
    enable_utc=True,
    result_expires=3600,
)

@app.task(bind=True)
def process_task(self, task_data):
    """
    Sample background task that processes data
    """
    try:
        task_id = self.request.id
        logger.info(f"Starting task {task_id} with data: {task_data}")
        
        # Simulate work
        total_steps = task_data.get('steps', 5)
        for i in range(total_steps):
            time.sleep(1)  # Simulate processing time
            
            # Update task progress
            self.update_state(
                state='PROGRESS',
                meta={
                    'current': i + 1,
                    'total': total_steps,
                    'status': f'Processing step {i + 1} of {total_steps}'
                }
            )
            logger.info(f"Task {task_id}: Completed step {i + 1}/{total_steps}")
        
        result = {
            'status': 'completed',
            'result': f"Processed {total_steps} steps successfully",
            'task_id': task_id,
            'processed_at': time.time()
        }
        
        logger.info(f"Task {task_id} completed successfully")
        return result
        
    except Exception as exc:
        logger.error(f"Task {self.request.id} failed: {str(exc)}")
        self.update_state(
            state='FAILURE',
            meta={
                'error': str(exc),
                'traceback': str(exc.__traceback__)
            }
        )
        raise

@app.task
def send_notification(message, recipient):
    """
    Sample notification task
    """
    logger.info(f"Sending notification to {recipient}: {message}")
    
    # Simulate sending email/notification
    time.sleep(2)
    
    return {
        'status': 'sent',
        'message': message,
        'recipient': recipient,
        'sent_at': time.time()
    }

@app.task
def cleanup_data():
    """
    Sample cleanup task that might run periodically
    """
    logger.info("Starting data cleanup task")
    
    # Simulate cleanup work
    time.sleep(3)
    
    return {
        'status': 'completed',
        'cleaned_items': 42,
        'completed_at': time.time()
    }

@app.task
def health_check():
    """
    Health check task for monitoring
    """
    return {
        'status': 'healthy',
        'timestamp': time.time(),
        'worker_id': os.getenv('HOSTNAME', 'unknown')
    }

if __name__ == '__main__':
    app.start()