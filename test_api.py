import urllib.request
import json

req = urllib.request.Request(
    'http://127.0.0.1:5000/api/register',
    data=json.dumps({"email": "test@test.com", "password": "password123"}).encode(),
    headers={'Content-Type': 'application/json'},
    method='POST'
)

try:
    response = urllib.request.urlopen(req)
    print("Response:", response.read().decode())
except Exception as e:
    print("Error:", e)