// Font families indexed by name
const FONTS = {
    serif: 'Georgia, "Times New Roman", serif',
    sans:  'system-ui, -apple-system, sans-serif',
    mono:  '"Courier New", Courier, monospace',
};

// ── Font & size preference ──

function setFont(name) {
    const content = document.getElementById('article-content');
    if (!content) return;
    content.style.fontFamily = FONTS[name] || FONTS.serif;
    document.querySelectorAll('.font-btn').forEach(b =>
        b.classList.toggle('active', b.dataset.font === name));
    localStorage.setItem('av-font', name);
}

function adjustSize(delta) {
    const content = document.getElementById('article-content');
    if (!content) return;
    const current = parseInt(localStorage.getItem('av-fontSize') || '16', 10);
    const next = Math.min(Math.max(current + delta * 2, 12), 28);
    content.style.fontSize = next + 'px';
    localStorage.setItem('av-fontSize', String(next));
}

function applyPrefs() {
    const font = localStorage.getItem('av-font') || 'serif';
    const size = localStorage.getItem('av-fontSize') || '16';
    setFont(font);
    const content = document.getElementById('article-content');
    if (content) content.style.fontSize = size + 'px';
}

// ── Modal open / close ──

function dismissModal() {
    const overlay = document.getElementById('modal-overlay');
    overlay.classList.remove('open');
    document.body.style.overflow = '';
    document.getElementById('modal-container').innerHTML = '';
}

// Click backdrop to close
document.getElementById('modal-overlay').addEventListener('click', function(e) {
    if (e.target === this) dismissModal();
});

// ESC to close
document.addEventListener('keydown', function(e) {
    if (e.key === 'Escape') dismissModal();
});

// Open modal and apply prefs after HTMX loads content
document.body.addEventListener('htmx:afterSwap', function(e) {
    if (e.detail.target.id === 'modal-container') {
        document.getElementById('modal-overlay').classList.add('open');
        document.body.style.overflow = 'hidden';
        applyPrefs();
    }
});

// ── Copy article content ──

function copyContent() {
    const content = document.getElementById('article-content');
    if (!content) return;
    navigator.clipboard.writeText(content.innerText).then(function() {
        const btn = document.getElementById('copy-btn');
        const original = btn.textContent;
        btn.textContent = 'Copied!';
        setTimeout(function() { btn.textContent = original; }, 2000);
    });
}

// ── Pill filter toggle ──

function togglePill(el, inputId) {
    const input = document.getElementById(inputId);
    const value = el.dataset.value;
    const pills = el.closest('.pills').querySelectorAll('.pill');

    if (input.value === value) {
        // Deselect: already active
        input.value = '';
        el.classList.remove('active');
    } else {
        // Select: deactivate siblings, activate this one
        pills.forEach(function(p) { p.classList.remove('active'); });
        input.value = value;
        el.classList.add('active');
    }

    // Fire HTMX request with current filter state
    const params = new URLSearchParams({
        q:        document.getElementById('search-input').value,
        site:     document.getElementById('site-filter').value,
        category: document.getElementById('cat-filter').value,
        offset:   '0',
    });
    // Remove empty params
    for (const [k, v] of [...params]) { if (!v) params.delete(k); }

    htmx.ajax('GET', '/articles?' + params.toString(), {
        target: '#feed',
        swap:   'innerHTML',
    });
}
