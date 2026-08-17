const form = document.getElementById('scrapeForm')
const statusDiv = document.getElementById('jobStatus')
const statusText = document.getElementById('statusText')
const leadsDiv = document.getElementById('leads')

form.addEventListener('submit', async (e) => {
  e.preventDefault()
  const data = new FormData(form)
  const body = {
    niche: data.get('niche'),
    location: data.get('location'),
    serviceDescription: data.get('serviceDescription') || '',
    limit: Number(data.get('limit')),
    depth: Number(data.get('depth')),
    concurrency: Number(data.get('concurrency')),
    extractEmail: data.get('extractEmail') === 'on'
  }
  const res = await fetch('/api/scrape', {method: 'POST', headers: {'Content-Type':'application/json'}, body: JSON.stringify(body)})
  const j = await res.json()
  if (j.jobId) {
    statusDiv.style.display = 'block'
    pollJob(j.jobId)
  }
})

async function pollJob(jobId){
  const url = '/api/jobs/' + jobId
  const iv = setInterval(async ()=>{
    try{
      const r = await fetch(url)
      if (!r.ok) throw new Error('job fetch failed')
      const s = await r.json()
      statusText.textContent = JSON.stringify(s, null, 2)
      if (s.status === 'completed'){
        clearInterval(iv)
        // fetch leads
        fetchLeads(jobId)
      }
      if (s.status === 'failed'){
        clearInterval(iv)
      }
    }catch(err){
      statusText.textContent = 'Error: '+err.message
      clearInterval(iv)
    }
  }, 2500)
}

async function fetchLeads(jobId){
  const r = await fetch('/data/leads/' + jobId + '.json')
  if (!r.ok){ leadsDiv.textContent = 'Leads not found'; return }
  const arr = await r.json()
  leadsDiv.innerHTML = '<h3>Leads</h3>' + arr.map(l=>`<div class="lead"><strong>${l.name}</strong> — ${l.address} — ${l.website || 'NO WEBSITE'} — ${l.rating} (${l.reviewCount})</div>`).join('')
}
