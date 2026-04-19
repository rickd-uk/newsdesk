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

document.getElementById('modal-overlay').addEventListener('click', function(e) {
    if (e.target === this) dismissModal();
});

document.addEventListener('keydown', function(e) {
    if (e.key === 'Escape') {
        if (document.getElementById('modal-overlay').classList.contains('open')) {
            dismissModal();
        } else if (document.getElementById('filter-panel').classList.contains('open')) {
            toggleFilters();
        }
    }
});

document.body.addEventListener('htmx:afterSwap', function(e) {
    if (e.detail.target.id === 'modal-container') {
        document.getElementById('modal-overlay').classList.add('open');
        document.body.style.overflow = 'hidden';
        applyPrefs();

        // Auto-mark as read
        const id = e.detail.elt.dataset.id;
        if (id) {
            const card = document.getElementById('card-' + id);
            if (card) card.classList.add('read');
            fetch('/article/' + id + '/read', { method: 'POST' });
        }
    }
});

// ── Mark unread ──

function markUnread() {
    const modal = document.querySelector('.modal');
    const id = modal && modal.dataset.articleId;
    if (!id) return;
    fetch('/article/' + id + '/unread', { method: 'POST' }).then(function() {
        const card = document.getElementById('card-' + id);
        if (card) card.classList.remove('read');
        const btn = document.getElementById('unread-btn');
        if (btn) { btn.textContent = '✓ Unread'; btn.disabled = true; }
    });
}

// ── Favorite toggle ──

function toggleFavorite() {
    const modal = document.querySelector('.modal');
    const id = modal && modal.dataset.articleId;
    if (!id) return;
    const btn = document.getElementById('fav-btn');
    const card = document.getElementById('card-' + id);
    const isFav = btn && btn.classList.contains('active');
    const action = isFav ? 'unfavorite' : 'favorite';
    fetch('/article/' + id + '/' + action, { method: 'POST' }).then(function() {
        if (btn) {
            btn.classList.toggle('active', !isFav);
            btn.textContent = !isFav ? '★ Saved' : '☆ Save';
        }
        if (card) card.classList.toggle('favorited', !isFav);
        // update star in card meta
        const meta = card && card.querySelector('.card-meta');
        if (meta) {
            const existing = meta.querySelector('.fav-star');
            if (!isFav && !existing) {
                const star = document.createElement('span');
                star.className = 'fav-star';
                star.textContent = '★';
                meta.prepend(star);
            } else if (isFav && existing) {
                existing.remove();
            }
        }
    });
}

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

// ── Compact view toggle ──

function toggleCompact() {
    const feed = document.getElementById('feed');
    const btn  = document.getElementById('view-btn');
    const on   = feed.classList.toggle('compact');
    btn.classList.toggle('active', on);
    localStorage.setItem('av-compact', on ? '1' : '');
}

function applyCompactPref() {
    if (localStorage.getItem('av-compact')) {
        document.getElementById('feed').classList.add('compact');
        document.getElementById('view-btn').classList.add('active');
    }
}

// ── Filter panel toggle ──

function toggleFilters() {
    const panel    = document.getElementById('filter-panel');
    const btn      = document.getElementById('filter-btn');
    const backdrop = document.getElementById('filter-backdrop');
    const open = panel.classList.toggle('open');
    btn.classList.toggle('active', open);
    backdrop.classList.toggle('open', open);
    document.body.style.overflow = open ? 'hidden' : '';
}

// ── Feed refresh ──

var _refreshTimer = null;

function debounceRefresh() {
    clearTimeout(_refreshTimer);
    _refreshTimer = setTimeout(fireFeedRefresh, 400);
}

function fireFeedRefresh() {
    const params = new URLSearchParams();
    const q = document.getElementById('search-input').value;
    if (q) params.set('q', q);

    document.querySelectorAll('#filter-panel [name]').forEach(function(el) {
        if (el.type === 'checkbox') {
            if (el.checked) params.append(el.name, el.value);
        } else if (el.value) {
            params.set(el.name, el.value);
        }
    });
    params.set('offset', '0');
    for (const [k, v] of [...params]) { if (!v) params.delete(k); }

    htmx.ajax('GET', '/articles?' + params.toString(), {
        target: '#feed', swap: 'innerHTML',
    });
    updateFilterBadge();
}

function updateFilterBadge() {
    const vals = [
        document.getElementById('site-filter')?.value,
        document.getElementById('cat-filter')?.value,
        document.getElementById('author-input')?.value,
        document.querySelector('[name="date_from"]')?.value,
        document.querySelector('[name="date_to"]')?.value,
    ].filter(Boolean).length;
    const badge = document.getElementById('filter-badge');
    if (!badge) return;
    badge.hidden = vals === 0;
    badge.textContent = vals || '';
}

// ── Category pill visibility filter ──

function filterCategoryPills(selectedSite) {
    // Show/hide individual pills
    document.querySelectorAll('#cat-pills .pill').forEach(function(pill) {
        if (!selectedSite) {
            pill.style.display = '';
        } else {
            const sites = (pill.dataset.sites || '').split(',').map(function(s) { return s.trim(); });
            pill.style.display = sites.includes(selectedSite) ? '' : 'none';
        }
    });

    // Hide subgroup wrappers whose pills are all hidden
    document.querySelectorAll('.cat-subgroup').forEach(function(sg) {
        const anyVisible = Array.from(sg.querySelectorAll('.pill')).some(function(p) {
            return p.style.display !== 'none';
        });
        sg.style.display = anyVisible ? '' : 'none';
    });

    // Hide whole cat-groups whose pills are all hidden
    document.querySelectorAll('.cat-group').forEach(function(g) {
        const anyVisible = Array.from(g.querySelectorAll('.pill')).some(function(p) {
            return p.style.display !== 'none';
        });
        g.style.display = anyVisible ? '' : 'none';
    });

    // Clear active category if it became hidden
    const catInput = document.getElementById('cat-filter');
    const activeCat = document.querySelector('#cat-pills .pill.active');
    if (activeCat && activeCat.style.display === 'none') {
        catInput.value = '';
        activeCat.classList.remove('active');
    }
}

// ── Pill filter toggle ──

function togglePill(el, inputId) {
    const input = document.getElementById(inputId);
    const value = el.dataset.value;
    // For site pills, siblings are in .pills; for cat pills, siblings span all cat-groups
    const container = inputId === 'cat-filter'
        ? document.getElementById('cat-pills')
        : el.closest('.pills');
    const pills = container ? container.querySelectorAll('.pill') : [];

    if (input.value === value) {
        input.value = '';
        el.classList.remove('active');
    } else {
        pills.forEach(function(p) { p.classList.remove('active'); });
        input.value = value;
        el.classList.add('active');
    }

    if (inputId === 'site-filter') {
        filterCategoryPills(input.value);
    }

    fireFeedRefresh();
}

function clearInput(id) {
    const el = document.getElementById(id);
    if (el) el.value = '';
    fireFeedRefresh();
}

function activateDateRange() {
    document.getElementById('date-range-row').classList.add('active');
    document.getElementById('date-from').focus();
}

function clearDates() {
    const from = document.getElementById('date-from');
    const to   = document.getElementById('date-to');
    if (from) from.value = '';
    if (to)   to.value   = '';
    document.getElementById('date-range-row').classList.remove('active');
    fireFeedRefresh();
}

// ── Init ──

document.addEventListener('DOMContentLoaded', function() {
    applyCompactPref();
    filterCategoryPills(document.getElementById('site-filter').value);
    updateFilterBadge();
    // Auto-open filter panel if any active filters on page load
    const hasFilters =
        document.getElementById('site-filter')?.value ||
        document.getElementById('cat-filter')?.value ||
        document.getElementById('author-input')?.value ||
        document.querySelector('[name="date_from"]')?.value ||
        document.querySelector('[name="date_to"]')?.value;
    if (hasFilters) {
        document.getElementById('filter-panel').classList.add('open');
        document.getElementById('filter-btn').classList.add('active');
        document.getElementById('filter-backdrop').classList.add('open');
    }
});
