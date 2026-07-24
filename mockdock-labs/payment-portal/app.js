const express = require('express');
const Stripe = require('stripe');
const axios = require('axios');

const app = express();
app.use(express.json());

const stripeSecret = process.env.STRIPE_SECRET_KEY || 'sk_test_mockdock12345';
const port = process.env.PORT || 3001;

// Initialize Stripe SDK
// EDUCATIONAL NOTE: In a normal environment, the SDK targets "https://api.stripe.com" directly.
// Normally, changing endpoints requires code updates (e.g. changing SDK baseUrls).
// Under MockDock, you DO NOT change the Stripe URL or config. 
// MockDock's DNS/SNI proxy intercepts the outbound traffic automatically!
const stripe = Stripe(stripeSecret);

app.get('/', (req, res) => {
  res.send(`
    <!DOCTYPE html>
    <html>
    <head>
      <title>Payment Portal (Stripe / Auth0 Interception)</title>
      <style>
        body { font-family: sans-serif; background: #0f172a; color: #f1f5f9; padding: 40px; display: flex; justify-content: center; }
        .card { background: #1e293b; padding: 24px; border-radius: 12px; box-shadow: 0 4px 6px -1px rgb(0 0 0 / 0.1); width: 450px; }
        button { padding: 10px 16px; border: none; border-radius: 6px; background: #10b981; color: #fff; cursor: pointer; font-size: 1rem; width: 100%; }
        pre { background: #0f172a; padding: 12px; border-radius: 6px; font-size: 0.85rem; color: #38bdf8; overflow-x: auto; }
      </style>
    </head>
    <body>
      <div class="card">
        <h2>MockDock Payment Portal</h2>
        <p>This panel triggers a real Stripe SDK call to <code>https://api.stripe.com/v1/charges</code>.</p>
        <button onclick="chargeCard()">Process Test Charge ($20.00)</button>
        <h3>Response Payload:</h3>
        <pre id="output">Click the button to process a charge...</pre>
      </div>
      <script>
        async function chargeCard() {
          const out = document.getElementById('output');
          out.innerText = 'Sending SDK charge request...';
          try {
            const res = await fetch('/api/charge', { method: 'POST' });
            const data = await res.json();
            out.innerText = JSON.stringify(data, null, 2);
          } catch(err) {
            out.innerText = 'Error: ' + err.message;
          }
        }
      </script>
    </body>
    </html>
  `);
});

// Charge Endpoint
app.post('/api/charge', async (req, res) => {
  // EDUCATIONAL NOTE:
  // 1. HOW NODE HANDLES TLS BY DEFAULT: Node.js has a built-in list of trusted root Certificate Authorities (CAs).
  //    When the Stripe SDK makes a call to api.stripe.com, Node verifies that the SSL cert is signed by an official root CA.
  // 2. WHAT MOCKDOCK DOES: When Stripe service is in "Ghost Mode", MockDock's proxy intercepts the connection request.
  //    It generates an SSL certificate for "api.stripe.com" on the fly, signed by MockDock's local Root CA.
  // 3. TRUSTING MOCKDOCK'S CA: By launching this container with NODE_EXTRA_CA_CERTS pointing to mockdock-ca.pem,
  //    Node.js appends MockDock's Root CA to its trusted pool. Thus, the TLS connection succeeds transparently!
  try {
    const charge = await stripe.charges.create({
      amount: 2000,
      currency: 'usd',
      source: 'tok_visa', // Mock token
      description: 'MockDock Sandbox Charge'
    });
    res.json({ source: 'Stripe Node SDK', charge });
  } catch (err) {
    res.status(500).json({ error: err.message });
  }
});

// Auth0 simulation endpoint using Axios
app.get('/api/auth-profile', async (req, res) => {
  try {
    const response = await axios.get('https://mockdock-tenant.auth0.com/userinfo', {
      headers: { 'Authorization': 'Bearer mock_token' }
    });
    res.json(response.data);
  } catch (err) {
    res.status(500).json({ error: err.message });
  }
});

app.listen(port, () => {
  console.log(`🚀 Payment Portal active on http://localhost:${port}`);
});
