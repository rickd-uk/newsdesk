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

function openAuthModal(mode) {
    const overlay = document.getElementById('auth-overlay');
    if (!overlay) return;
    if (mode === 'signup' || mode === 'login') setAuthMode(mode);
    overlay.classList.add('open');
    document.body.style.overflow = 'hidden';
}

function closeAuthModal() {
    const overlay = document.getElementById('auth-overlay');
    if (!overlay) return;
    overlay.classList.remove('open');
    document.body.style.overflow = document.getElementById('modal-overlay').classList.contains('open') ? 'hidden' : '';
}

function handleAuthOverlayClick(e) {
    if (e.target.id === 'auth-overlay') closeAuthModal();
}

function setAuthMode(mode) {
    const signupTab = document.getElementById('auth-tab-signup');
    const loginTab = document.getElementById('auth-tab-login');
    const signupPanel = document.getElementById('auth-panel-signup');
    const loginPanel = document.getElementById('auth-panel-login');
    if (!signupTab || !loginTab || !signupPanel || !loginPanel) return;

    const login = mode === 'login';
    signupTab.classList.toggle('active', !login);
    loginTab.classList.toggle('active', login);
    signupPanel.classList.toggle('hidden', login);
    loginPanel.classList.toggle('hidden', !login);
}

document.getElementById('modal-overlay').addEventListener('click', function(e) {
    if (e.target === this) dismissModal();
});

document.addEventListener('keydown', function(e) {
    if (e.key === 'Escape') {
        if (document.getElementById('modal-overlay').classList.contains('open')) {
            dismissModal();
        } else if (document.getElementById('auth-overlay').classList.contains('open')) {
            closeAuthModal();
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

        var modalEl = document.querySelector('#modal-container .modal');
        var id = modalEl && modalEl.dataset.articleId;
        if (id) setupReadSentinel(id);
    }
});

// Mark as read once the user has scrolled to the bottom of the article.
function setupReadSentinel(id) {
    var content = document.getElementById('article-content');
    var modal   = document.querySelector('.modal');
    if (!content || !modal) return;
    if (modal.dataset.authenticated !== '1') return;

    // Desktop: article-content scrolls (max-height + overflow-y: auto).
    // Mobile:  .modal itself scrolls (max-height: 93vh; overflow-y: auto).
    var isMobile = window.matchMedia('(max-width: 600px)').matches;
    var scroller = isMobile ? modal : content;
    var done     = false;

    function doMark() {
        if (done) return;
        done = true;
        scroller.removeEventListener('scroll', onScroll);

        var card = document.getElementById('card-' + id);
        if (card && !card.classList.contains('read')) {
            card.classList.add('read');
            var meta = card.querySelector('.card-meta');
            if (meta && !meta.querySelector('.read-badge')) {
                var badge = document.createElement('span');
                badge.className = 'read-badge';
                badge.title = 'Read';
                badge.textContent = '✓ Read';
                meta.prepend(badge);
            }
        }
        var btn = document.getElementById('unread-btn');
        if (btn) btn.disabled = false;
        fetch('/article/' + id + '/read', { method: 'POST' });
    }

    function atBottom() {
        var gap = scroller.scrollHeight - scroller.scrollTop - scroller.clientHeight;
        return gap <= 60;
    }

    function onScroll() { if (atBottom()) doMark(); }

    scroller.addEventListener('scroll', onScroll);
    // Retry a few times to handle font/size reflow after applyPrefs()
    var checks = [100, 300, 600];
    checks.forEach(function(ms) {
        setTimeout(function() { if (atBottom()) doMark(); }, ms);
    });
}

// ── Mark unread ──

function markUnread() {
    var modal = document.querySelector('.modal');
    var id = modal && modal.dataset.articleId;
    if (!id || modal.dataset.authenticated !== '1') return;
    fetch('/article/' + id + '/unread', { method: 'POST' }).then(function() {
        var card = document.getElementById('card-' + id);
        if (card) {
            card.classList.remove('read');
            var badge = card.querySelector('.read-badge');
            if (badge) badge.remove();
        }
        var btn = document.getElementById('unread-btn');
        if (btn) btn.disabled = true;
    });
}

// ── Favorite toggle ──

function toggleFavorite() {
    const modal = document.querySelector('.modal');
    const id = modal && modal.dataset.articleId;
    if (!id || modal.dataset.authenticated !== '1') return;
    const btn = document.getElementById('fav-btn');
    const card = document.getElementById('card-' + id);
    const isFav = btn && btn.classList.contains('active');
    const action = isFav ? 'unfavorite' : 'favorite';
    fetch('/article/' + id + '/' + action, { method: 'POST' }).then(function() {
        if (btn) {
            btn.classList.toggle('active', !isFav);
            btn.textContent = !isFav ? '★' : '☆';
            btn.title = !isFav ? 'Unsave' : 'Save';
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
    if (q.trim().length >= 2) addHistory('av-recent-searches', q.trim());
    const authorVal = document.getElementById('author-input').value;
    if (authorVal.trim().length >= 2) addHistory('av-recent-authors', authorVal.trim());

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

// ── Recent history (search & author) ──

function getHistory(key) {
    try { return JSON.parse(localStorage.getItem(key) || '[]'); }
    catch (e) { return []; }
}

function addHistory(key, value) {
    if (!value) return;
    var list = getHistory(key).filter(function(s) { return s !== value; });
    list.unshift(value);
    localStorage.setItem(key, JSON.stringify(list.slice(0, 12)));
}

var _activeDrop = null;

function showSuggestions(type) {
    var isSearch = type === 'search';
    var inputId  = isSearch ? 'search-input' : 'author-input';
    var dropId   = isSearch ? 'search-suggestions' : 'author-suggestions';
    var key      = isSearch ? 'av-recent-searches' : 'av-recent-authors';

    var input = document.getElementById(inputId);
    var drop  = document.getElementById(dropId);
    var list  = getHistory(key);

    if (!list.length) { drop.hidden = true; return; }

    drop.innerHTML = '';
    list.forEach(function(term) {
        var btn = document.createElement('button');
        btn.type = 'button';
        btn.className = 'suggestion-item';
        btn.textContent = term;
        btn.addEventListener('mousedown', function(e) {
            e.preventDefault();
            input.value = term;
            hideSuggestions();
            fireFeedRefresh();
        });
        drop.appendChild(btn);
    });

    var clr = document.createElement('button');
    clr.type = 'button';
    clr.className = 'suggestions-clear';
    clr.textContent = 'Clear history';
    clr.addEventListener('mousedown', function(e) {
        e.preventDefault();
        localStorage.removeItem(key);
        hideSuggestions();
    });
    drop.appendChild(clr);

    if (isSearch) {
        // Position fixed below the search input
        var rect = input.getBoundingClientRect();
        drop.style.top   = (rect.bottom + 4) + 'px';
        drop.style.left  = rect.left + 'px';
        drop.style.width = rect.width + 'px';
    }

    drop.hidden = false;
    _activeDrop = drop;
}

function hideSuggestions() {
    if (_activeDrop) { _activeDrop.hidden = true; _activeDrop = null; }
}

// ── Init ──

document.addEventListener('DOMContentLoaded', function() {
    applyCompactPref();
    filterCategoryPills(document.getElementById('site-filter').value);
    updateFilterBadge();
    if (document.getElementById('auth-overlay')?.classList.contains('preopen')) {
        openAuthModal(document.getElementById('auth-tab-login')?.classList.contains('active') ? 'login' : 'signup');
        document.getElementById('auth-overlay').classList.remove('preopen');
    }

    // Wire up history dropdowns
    var searchInput = document.getElementById('search-input');
    var authorInput = document.getElementById('author-input');

    searchInput.addEventListener('focus', function() { showSuggestions('search'); });
    searchInput.addEventListener('blur',  function() { setTimeout(hideSuggestions, 150); });
    authorInput.addEventListener('focus', function() { showSuggestions('author'); });
    authorInput.addEventListener('blur',  function() { setTimeout(hideSuggestions, 150); });

    // Auto-open filter panel if any active filters on page load
    const hasFilters =
        document.getElementById('site-filter')?.value ||
        document.getElementById('cat-filter')?.value ||
        authorInput?.value ||
        document.querySelector('[name="date_from"]')?.value ||
        document.querySelector('[name="date_to"]')?.value;
    if (hasFilters) {
        document.getElementById('filter-panel').classList.add('open');
        document.getElementById('filter-btn').classList.add('active');
        document.getElementById('filter-backdrop').classList.add('open');
    }
});
