(function () {
	var DEBOUNCE_MS = 50;

	var overlay, input, list;
	var results = [];
	var selected = 0;
	var timer = null;
	var requestId = 0;
	var opener = null;

	var isMac = /Mac|iPhone|iPad/.test(navigator.platform || navigator.userAgent);
	document.querySelectorAll('.picker-hint').forEach(function (el) {
		el.textContent = isMac ? '⌥O' : 'Alt+O';
	});

	function build() {
		overlay = document.createElement('div');
		overlay.className = 'picker-overlay';
		overlay.hidden = true;

		var box = document.createElement('div');
		box.className = 'picker';
		box.setAttribute('role', 'dialog');
		box.setAttribute('aria-modal', 'true');
		box.setAttribute('aria-label', 'Search files');

		input = document.createElement('input');
		input.type = 'text';
		input.className = 'picker-input';
		input.placeholder = 'Search files…';
		input.setAttribute('autocomplete', 'off');
		input.setAttribute('spellcheck', 'false');
		input.setAttribute('aria-label', 'Search files');

		list = document.createElement('ul');
		list.className = 'picker-results';

		box.appendChild(input);
		box.appendChild(list);
		overlay.appendChild(box);
		document.body.appendChild(overlay);

		overlay.addEventListener('mousedown', function (e) {
			if (e.target === overlay) {
				e.preventDefault();
				close();
			}
		});
		input.addEventListener('input', function () {
			clearTimeout(timer);
			timer = setTimeout(search, DEBOUNCE_MS);
		});
		input.addEventListener('keydown', onInputKey);
	}

	function isOpen() {
		return overlay && !overlay.hidden;
	}

	function open() {
		if (!overlay) build();
		opener = document.activeElement;
		overlay.hidden = false;
		input.value = '';
		results = [];
		list.textContent = '';
		input.focus();
		search();
	}

	function close() {
		clearTimeout(timer);
		requestId++;
		overlay.hidden = true;
		if (opener && opener.focus) opener.focus();
	}

	function search() {
		var id = ++requestId;
		fetch('/_mdserver/files?q=' + encodeURIComponent(input.value))
			.then(function (res) {
				if (!res.ok) throw new Error('HTTP ' + res.status);
				return res.json();
			})
			.then(function (data) {
				if (id !== requestId) return;
				results = data;
				selected = 0;
				render();
			})
			.catch(function () {
				if (id !== requestId) return;
				results = [];
				render('Search unavailable');
			});
	}

	function render(message) {
		list.textContent = '';
		if (results.length === 0) {
			var empty = document.createElement('li');
			empty.className = 'picker-empty';
			empty.textContent = message || 'No matching files';
			list.appendChild(empty);
			return;
		}
		results.forEach(function (result, i) {
			var li = document.createElement('li');
			li.className = 'picker-result' + (i === selected ? ' selected' : '');
			li.appendChild(highlight(result.path, result.matches));
			li.addEventListener('mousedown', function (e) {
				e.preventDefault();
				go(result);
			});
			list.appendChild(li);
		});
	}

	// highlight returns the path with matched characters wrapped in <mark>. Match
	// positions are characters, not UTF-16 units, so split the path by code point.
	function highlight(path, matches) {
		var frag = document.createDocumentFragment();
		var matched = {};
		(matches || []).forEach(function (m) { matched[m] = true; });
		var run = '';
		var runMatched = false;
		function flush() {
			if (!run) return;
			var node = runMatched ? document.createElement('mark') : document.createElement('span');
			node.textContent = run;
			frag.appendChild(node);
			run = '';
		}
		Array.from(path).forEach(function (ch, i) {
			var isMatch = !!matched[i];
			if (isMatch !== runMatched) {
				flush();
				runMatched = isMatch;
			}
			run += ch;
		});
		flush();
		return frag;
	}

	function go(result) {
		window.location.href = result.href;
	}

	function move(delta) {
		if (results.length === 0) return;
		selected = (selected + delta + results.length) % results.length;
		var items = list.querySelectorAll('.picker-result');
		items.forEach(function (el, i) { el.classList.toggle('selected', i === selected); });
		items[selected].scrollIntoView({ block: 'nearest' });
	}

	function onInputKey(e) {
		if (e.key === 'ArrowDown' || (e.ctrlKey && e.key === 'n')) {
			e.preventDefault();
			move(1);
		} else if (e.key === 'ArrowUp' || (e.ctrlKey && e.key === 'p')) {
			e.preventDefault();
			move(-1);
		} else if (e.key === 'Enter') {
			e.preventDefault();
			if (!e.isComposing && results[selected]) go(results[selected]);
		}
	}

	// Option-O types a character on macOS, so match the physical key.
	document.addEventListener('keydown', function (e) {
		if (e.altKey && !e.ctrlKey && !e.metaKey && !e.shiftKey && e.code === 'KeyO') {
			e.preventDefault();
			if (isOpen()) close(); else open();
		}
	}, true);

	// Escape closes the picker even when focus has left the input.
	document.addEventListener('keydown', function (e) {
		if (e.key === 'Escape' && isOpen()) {
			e.preventDefault();
			close();
		}
	});

	document.addEventListener('click', function (e) {
		if (e.target.closest && e.target.closest('[data-picker-open]')) {
			if (!isOpen()) open();
		}
	});
})();
