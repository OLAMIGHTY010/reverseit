from flask import Flask, request, jsonify
from parser import parse_alert

app = Flask(__name__)

@app.route('/parse-alert', methods=['POST'])
def parse_alert_endpoint():
    data = request.get_json()
    if not data or 'alertText' not in data:
        return jsonify({'error': 'alertText is required'}), 400
    
    alert_text = data['alertText']
    try:
        parsed = parse_alert(alert_text)
        return jsonify(parsed)
    except Exception as e:
        return jsonify({'error': str(e)}), 500

@app.route('/', methods=['GET'])
def health_check():
    return jsonify({'status': 'ReverseIt Alert Parser API is running'})

if __name__ == '__main__':
    app.run(host='0.0.0.0', port=5000, debug=True)
