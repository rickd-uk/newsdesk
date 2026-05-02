// Font families indexed by name
const FONTS = {
    serif: 'Georgia, "Times New Roman", serif',
    sans:  'system-ui, -apple-system, sans-serif',
    mono:  '"Courier New", Courier, monospace',
};

function filterDrawerLocksPage() {
    const panel = document.getElementById('filter-panel');
    return !!(panel && panel.classList.contains('open') && window.matchMedia('(max-width: 600px)').matches);
}

function updateBodyScrollLock() {
    const modalOpen = document.getElementById('modal-overlay')?.classList.contains('open');
    const authOpen = document.getElementById('auth-overlay')?.classList.contains('open');
    document.body.style.overflow = (modalOpen || authOpen || filterDrawerLocksPage()) ? 'hidden' : '';
}

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

function ensureModalStateBadge(id, className, text) {
    var row = document.querySelector('.modal-state-row');
    if (!row) return;
    var badge = document.getElementById(id);
    if (!badge) {
        badge = document.createElement('span');
        badge.id = id;
        badge.className = 'modal-state-badge ' + className;
        row.appendChild(badge);
    }
    badge.textContent = text;
}

function removeModalStateBadge(id) {
    var badge = document.getElementById(id);
    if (badge) badge.remove();
}

function filterChecked(name) {
    return !!document.querySelector('[name="' + name + '"]')?.checked;
}

function removeCardIfExcludedByStateFilters(card) {
    if (!card) return false;

    const hiddenByReadState =
        (filterChecked('hide_read') && card.classList.contains('read')) ||
        (filterChecked('reads_only') && !card.classList.contains('read'));
    const hiddenByFavoriteState =
        filterChecked('favorites_only') && !card.classList.contains('favorited');
    const hiddenByArchiveState =
        (filterChecked('archived_only') && !card.classList.contains('archived')) ||
        (!filterChecked('archived_only') && card.classList.contains('archived'));

    if (hiddenByReadState || hiddenByFavoriteState || hiddenByArchiveState) {
        card.remove();
        return true;
    }
    return false;
}

// ── Modal open / close ──

function dismissModal() {
    const overlay = document.getElementById('modal-overlay');
    if (!overlay) return;
    overlay.classList.remove('open');
    const container = document.getElementById('modal-container');
    if (container) container.innerHTML = '';
    updateBodyScrollLock();
}

function openAuthModal(mode) {
    const overlay = document.getElementById('auth-overlay');
    if (!overlay) return;
    if (mode === 'signup' || mode === 'login') setAuthMode(mode);
    overlay.classList.add('open');
    updateBodyScrollLock();
}

function closeAuthModal() {
    const overlay = document.getElementById('auth-overlay');
    if (!overlay) return;
    overlay.classList.remove('open');
    updateBodyScrollLock();
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
        if (document.getElementById('modal-overlay')?.classList.contains('open')) {
            dismissModal();
        } else if (document.getElementById('auth-overlay')?.classList.contains('open')) {
            closeAuthModal();
        } else if (document.getElementById('filter-panel')?.classList.contains('open')) {
            toggleFilters();
        }
    }
});

document.body.addEventListener('htmx:afterSwap', function(e) {
    if (e.detail.target.id === 'modal-container') {
        document.getElementById('modal-overlay').classList.add('open');
        updateBodyScrollLock();
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
        modal.classList.add('is-read');
        ensureModalStateBadge('modal-read-indicator', 'read', '✓ Read');
        var btn = document.getElementById('unread-btn');
        if (btn) btn.disabled = false;
        removeCardIfExcludedByStateFilters(card);
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
        modal.classList.remove('is-read');
        removeModalStateBadge('modal-read-indicator');
        var btn = document.getElementById('unread-btn');
        if (btn) btn.disabled = true;
        removeCardIfExcludedByStateFilters(card);
        setupReadSentinel(id);
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
        modal.classList.toggle('is-favorite', !isFav);
        if (!isFav) {
            ensureModalStateBadge('modal-fav-indicator', 'favorite', '★ Favorite');
        } else {
            removeModalStateBadge('modal-fav-indicator');
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
        removeCardIfExcludedByStateFilters(card);
    });
}

// ── Archive toggle ──

function toggleArchive() {
    const modal = document.querySelector('.modal');
    const id = modal && modal.dataset.articleId;
    if (!id || modal.dataset.authenticated !== '1') return;
    const btn = document.getElementById('archive-btn');
    const card = document.getElementById('card-' + id);
    const isArchived = btn && btn.classList.contains('active');
    const action = isArchived ? 'unarchive' : 'archive';
    fetch('/article/' + id + '/' + action, { method: 'POST' }).then(function() {
        if (btn) {
            btn.classList.toggle('active', !isArchived);
            btn.title = !isArchived ? 'Unarchive' : 'Archive';
        }
        modal.classList.toggle('is-archived', !isArchived);
        if (!isArchived) {
            ensureModalStateBadge('modal-archive-indicator', 'archived', '▣ Archived');
        } else {
            removeModalStateBadge('modal-archive-indicator');
        }
        if (card) {
            card.classList.toggle('archived', !isArchived);
            const meta = card.querySelector('.card-meta');
            if (meta) {
                const existing = meta.querySelector('.archive-badge');
                if (!isArchived && !existing) {
                    const badge = document.createElement('span');
                    badge.className = 'archive-badge';
                    badge.title = 'Archived';
                    badge.textContent = '▣';
                    meta.prepend(badge);
                } else if (isArchived && existing) {
                    existing.remove();
                }
            }
            removeCardIfExcludedByStateFilters(card);
        }
    });
}

// ── Article notes ──

function toggleArticleNote() {
    const panel = document.getElementById('note-panel');
    const btn = document.getElementById('note-toggle-btn');
    if (!panel || !btn || btn.disabled) return;
    const open = panel.classList.toggle('hidden') === false;
    btn.classList.toggle('active', open);
    btn.title = open ? 'Hide note' : 'Add note';
    if (open) {
        const input = document.getElementById('article-note-input');
        if (input) input.focus();
    }
}

function setNoteStatus(text) {
    const status = document.getElementById('note-save-status');
    if (status) status.textContent = text;
}

function updateCardNote(id, note) {
    const card = document.getElementById('card-' + id);
    if (!card) return;
    let noteEl = card.querySelector('.card-note');
    const notesHidden = document.querySelector('[name="hide_notes"]')?.checked;
    if (!note || notesHidden) {
        if (noteEl) noteEl.remove();
        return;
    }
    if (!noteEl) {
        noteEl = document.createElement('div');
        noteEl.className = 'card-note';
        card.appendChild(noteEl);
    }
    noteEl.textContent = 'Note: ' + note;
}

function saveArticleNote() {
    const modal = document.querySelector('.modal');
    const id = modal && modal.dataset.articleId;
    const input = document.getElementById('article-note-input');
    if (!id || !input || modal.dataset.authenticated !== '1') return;

    setNoteStatus('Saving...');
    const body = new URLSearchParams();
    body.set('note', input.value);
    fetch('/article/' + id + '/note', {
        method: 'POST',
        headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
        body: body.toString(),
    }).then(function(resp) {
        if (!resp.ok) throw new Error('save failed');
        const note = input.value.trim();
        updateCardNote(id, note);
        const clearBtn = document.querySelector('.note-clear-btn');
        if (clearBtn) clearBtn.disabled = !note;
        const btn = document.getElementById('note-toggle-btn');
        if (btn) btn.title = note ? 'Hide note' : 'Add note';
        setNoteStatus(note ? 'Saved' : 'Cleared');
        setTimeout(function() { setNoteStatus(''); }, 1800);
    }).catch(function() {
        setNoteStatus('Save failed');
    });
}

function clearArticleNote() {
    const input = document.getElementById('article-note-input');
    if (!input) return;
    input.value = '';
    saveArticleNote();
}

// ── Copy article content ──

function copyContent() {
    const content = document.getElementById('article-content');
    if (!content) return;
    navigator.clipboard.writeText(content.innerText).then(function() {
        const btn = document.getElementById('copy-btn');
        const original = btn.textContent;
        btn.textContent = '✓';
        setTimeout(function() { btn.textContent = original; }, 2000);
    });
}

// ── Compact view toggle ──

function toggleCompact() {
    const feed = document.getElementById('feed');
    const btn  = document.getElementById('view-btn');
    if (!feed || !btn) return;
    const current = feed.classList.contains('title-only') ? 2 : feed.classList.contains('compact') ? 1 : 0;
    setFeedViewMode((current + 1) % 3);
}

function applyCompactPref() {
    var stored = localStorage.getItem('av-view-mode');
    if (stored === null && localStorage.getItem('av-compact')) stored = '1';
    setFeedViewMode(parseInt(stored || '0', 10) || 0);
}

function setFeedViewMode(mode) {
    const feed = document.getElementById('feed');
    const btn = document.getElementById('view-btn');
    if (!feed || !btn) return;
    mode = Math.min(Math.max(mode, 0), 2);
    feed.classList.toggle('compact', mode === 1);
    feed.classList.toggle('title-only', mode === 2);
    btn.classList.toggle('active', mode > 0);
    btn.title = ['Normal view', 'Compact view', 'Title-only view'][mode];
    localStorage.setItem('av-view-mode', String(mode));
    localStorage.setItem('av-compact', mode === 1 ? '1' : '');
}

// ── Filter panel toggle ──

function toggleFilters() {
    const panel    = document.getElementById('filter-panel');
    const btn      = document.getElementById('filter-btn');
    const backdrop = document.getElementById('filter-backdrop');
    if (!panel || !btn || !backdrop) return;
    const open = panel.classList.toggle('open');
    btn.classList.toggle('active', open);
    backdrop.classList.toggle('open', open);
    updateBodyScrollLock();
}

// ── Feed refresh ──

var _refreshTimer = null;

const DEFAULT_DATE_FROM = '2000-01-01';

function todayISODate() {
    const now = new Date();
    const local = new Date(now.getFullYear(), now.getMonth(), now.getDate());
    return local.toISOString().slice(0, 10);
}

function applyDefaultDateRange() {
    const from = document.getElementById('date-from');
    const to = document.getElementById('date-to');
    const row = document.getElementById('date-range-row');
    if (!from || !to || !row) return;

    if (!from.value) from.value = DEFAULT_DATE_FROM;
    if (!to.value) to.value = todayISODate();
    row.classList.add('active');
}

function debounceRefresh() {
    clearTimeout(_refreshTimer);
    _refreshTimer = setTimeout(fireFeedRefresh, 400);
}

function fireFeedRefresh() {
    const params = new URLSearchParams();
    const readsOnly = document.querySelector('[name="reads_only"]');
    const hideRead = document.querySelector('[name="hide_read"]');
    if (readsOnly?.checked && hideRead) hideRead.checked = false;
    const searchInput = document.getElementById('search-input');
    if (!searchInput) return;
    const q = searchInput.value;
    if (q) params.set('q', q);
    if (q.trim().length >= 2) addHistory('av-recent-searches', q.trim());
    const authorVal = document.getElementById('author-input')?.value || '';
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

// ── Mobile site picker ──

function getSiteOptions() {
    return Array.from(document.querySelectorAll('#site-pills .pill')).map(function(pill) {
        return pill.dataset.value || pill.textContent.trim();
    }).filter(Boolean);
}

function syncSiteControls(selectedSite) {
    document.querySelectorAll('#site-pills .pill').forEach(function(pill) {
        pill.classList.toggle('active', (pill.dataset.value || '') === selectedSite);
    });

    const siteInput = document.getElementById('site-search');
    if (siteInput && document.activeElement !== siteInput) {
        siteInput.value = selectedSite;
    }

    document.querySelector('.site-all-btn')?.classList.toggle('active', !selectedSite);
}

function hideSiteOptions() {
    const menu = document.getElementById('site-options-menu');
    if (menu) menu.hidden = true;
}

function renderSiteOptions(query) {
    const menu = document.getElementById('site-options-menu');
    if (!menu) return;

    const selectedSite = document.getElementById('site-filter')?.value || '';
    const normalized = (query || '').trim().toLowerCase();
    const matches = getSiteOptions().filter(function(site) {
        return !normalized || site.toLowerCase().includes(normalized);
    });

    menu.innerHTML = '';
    if (!matches.length) {
        const empty = document.createElement('div');
        empty.className = 'site-option site-option-empty';
        empty.textContent = 'No matches';
        menu.appendChild(empty);
    } else {
        matches.forEach(function(site) {
            const btn = document.createElement('button');
            btn.type = 'button';
            btn.className = 'site-option';
            btn.classList.toggle('active', site === selectedSite);
            btn.textContent = site;
            btn.addEventListener('mousedown', function(e) {
                e.preventDefault();
                selectMobileSite(site);
            });
            menu.appendChild(btn);
        });
    }

    menu.hidden = false;
}

function selectMobileSite(site) {
    const input = document.getElementById('site-filter');
    const search = document.getElementById('site-search');
    if (!input) return;

    input.value = site || '';
    if (search) search.value = site || '';
    syncSiteControls(input.value);
    filterCategoryPills(input.value);
    hideSiteOptions();
    fireFeedRefresh();
}

function clearMobileSiteSearch() {
    const input = document.getElementById('site-filter');
    if (!input) return;

    input.value = '';
    syncSiteControls('');
    filterCategoryPills('');
    fireFeedRefresh();
    renderSiteOptions('');
}

// ── Category pill visibility filter ──

function filterCategoryPills(selectedSite) {
    const categorySection = document.getElementById('category-section');
    const catInput = document.getElementById('cat-filter');
    if (categorySection) categorySection.hidden = !selectedSite;

    if (!selectedSite) {
        if (catInput) catInput.value = '';
        document.querySelectorAll('#cat-pills .pill.active').forEach(function(pill) {
            pill.classList.remove('active');
        });
        document.querySelectorAll('#cat-pills .pill, .cat-subgroup, .cat-group').forEach(function(el) {
            el.style.display = 'none';
        });
        return;
    }

    // Show/hide individual pills
    document.querySelectorAll('#cat-pills .pill').forEach(function(pill) {
        const sites = (pill.dataset.sites || '').split(',').map(function(s) { return s.trim(); });
        pill.style.display = sites.includes(selectedSite) ? '' : 'none';
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

        const label = Array.from(g.children).find(function(el) {
            return el.classList && el.classList.contains('cat-group-label');
        });
        if (label) {
            label.hidden = false;
        }
        if (label && anyVisible) {
            const directPills = Array.from(g.children).filter(function(el) {
                return el.classList && el.classList.contains('pill') && el.style.display !== 'none';
            });
            const visibleSubgroups = Array.from(g.children).filter(function(el) {
                return el.classList && el.classList.contains('cat-subgroup') && el.style.display !== 'none';
            });
            const labelText = label.textContent.replace(/:\s*$/, '').trim().toLowerCase();
            const pillText = directPills.length === 1 ? directPills[0].textContent.trim().toLowerCase() : '';
            label.hidden = visibleSubgroups.length === 0 && directPills.length === 1 && labelText === pillText;
        }
    });

    // Clear active category if it became hidden
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
        syncSiteControls(input.value);
    }

    fireFeedRefresh();
}

function clearInput(id) {
    const el = document.getElementById(id);
    if (el) el.value = '';
    fireFeedRefresh();
}

function activateDateRange() {
    applyDefaultDateRange();
    document.getElementById('date-range-row')?.classList.add('active');
    document.getElementById('date-from')?.focus();
}

function clearDates() {
    const from = document.getElementById('date-from');
    const to   = document.getElementById('date-to');
    if (from) from.value = DEFAULT_DATE_FROM;
    if (to)   to.value   = todayISODate();
    document.getElementById('date-range-row')?.classList.add('active');
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
    applyDefaultDateRange();
    filterCategoryPills(document.getElementById('site-filter')?.value || '');
    syncSiteControls(document.getElementById('site-filter')?.value || '');
    updateFilterBadge();
    if (document.getElementById('auth-overlay')?.classList.contains('preopen')) {
        openAuthModal(document.getElementById('auth-tab-login')?.classList.contains('active') ? 'login' : 'signup');
        document.getElementById('auth-overlay').classList.remove('preopen');
    }

    // Wire up history dropdowns
    var searchInput = document.getElementById('search-input');
    var authorInput = document.getElementById('author-input');
    var siteSearchInput = document.getElementById('site-search');

    if (searchInput) {
        searchInput.addEventListener('focus', function() { showSuggestions('search'); });
        searchInput.addEventListener('blur',  function() { setTimeout(hideSuggestions, 150); });
    }
    if (authorInput) {
        authorInput.addEventListener('focus', function() { showSuggestions('author'); });
        authorInput.addEventListener('blur',  function() { setTimeout(hideSuggestions, 150); });
    }
    if (siteSearchInput) {
        siteSearchInput.addEventListener('focus', function() { renderSiteOptions(siteSearchInput.value); });
        siteSearchInput.addEventListener('input', function() {
            if (!siteSearchInput.value.trim()) {
                clearMobileSiteSearch();
                return;
            }
            renderSiteOptions(siteSearchInput.value);
        });
        siteSearchInput.addEventListener('keydown', function(e) {
            if (e.key === 'Escape') {
                hideSiteOptions();
                siteSearchInput.blur();
                return;
            }
            if (e.key !== 'Enter') return;
            const query = siteSearchInput.value.trim().toLowerCase();
            const match = getSiteOptions().find(function(site) {
                return site.toLowerCase() === query;
            });
            if (match) {
                e.preventDefault();
                selectMobileSite(match);
            }
        });
        siteSearchInput.addEventListener('blur', function() {
            setTimeout(function() {
                siteSearchInput.value = document.getElementById('site-filter')?.value || '';
                hideSiteOptions();
            }, 150);
        });
    }
    document.addEventListener('click', function(e) {
        if (!e.target.closest('.site-mobile-picker')) hideSiteOptions();
    });

    // Auto-open filter panel if any active filters on page load
    const hasFilters =
        document.getElementById('site-filter')?.value ||
        document.getElementById('cat-filter')?.value ||
        authorInput?.value ||
        document.querySelector('[name="date_from"]')?.value ||
        document.querySelector('[name="date_to"]')?.value;
    if (hasFilters) {
        document.getElementById('filter-panel')?.classList.add('open');
        document.getElementById('filter-btn')?.classList.add('active');
        document.getElementById('filter-backdrop')?.classList.add('open');
        updateBodyScrollLock();
    }
});

window.addEventListener('resize', updateBodyScrollLock);
