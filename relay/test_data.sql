-- Test Data for Keywords Extractor
-- Run with: psql $DATABASE_URL -f test_data.sql

BEGIN;

-- Clean up old test data
DELETE FROM event_logs WHERE issue_id >= 9000000000000;
DELETE FROM issues WHERE id >= 9000000000000;

-- Insert test issue with realistic content
INSERT INTO issues (
    id, project_id, external_id, provider,
    title, description, status, created_at, updated_at
) VALUES (
    9000000000001,
    1,
    'TEST-KEYWORDS-1',
    'linear',
    'Add Twilio SMS support for notifications',
    E'We need to integrate Twilio to send SMS notifications to users.\n\nRequirements:\n- Support for US and international numbers\n- Handle rate limiting (Twilio has strict limits)\n- Retry failed messages with exponential backoff\n- Store delivery status in notifications table\n\nRelated: The existing EmailProvider interface might be a good pattern to follow.',
    'queued',
    NOW(),
    NOW()
);

-- Insert corresponding event
INSERT INTO event_logs (
    id, project_id, issue_id, event_type, source,
    payload, created_at
) VALUES (
    9100000000001,
    1,
    9000000000001,
    'issue.created',
    'linear',
    '{"action": "create"}',
    NOW()
);

COMMIT;

-- Expected keywords (for validation):
-- High weight: twilio, sms, notifications, emailprovider, rate limiting
-- Medium weight: international, retry, exponential backoff
-- Lower weight: delivery status, notifications table

\echo 'Test issue inserted. Issue ID: 9000000000001'
\echo 'Start the worker to process.'
\echo ''
\echo 'To check results:'
\echo '  SELECT id, title, jsonb_pretty(keywords) FROM issues WHERE id = 9000000000001;'
