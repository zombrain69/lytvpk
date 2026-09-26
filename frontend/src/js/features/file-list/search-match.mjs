// 检索的"明确匹配"层：把"为什么这行被搜出来"变成看得见的东西。
//
// 后端 SearchVPKFiles 用的是 fuzzyMatch（query 的字符按顺序出现在字段里即命中），
// 所以这里保持同一套语义：
//   1. 能连续命中（子串）就高亮整段 —— 最直观；
//   2. 只能"按顺序命中"（例如 "ak" 命中 "A.K.47"）就逐字高亮 —— 与后端判定一致，
//      不会出现"搜出来了却没有任何高亮"的困惑；
//   3. 同时给出命中的字段名（标题 / 文件名 / 一级标签 / 子标签 / 主体 …），
//      用 chip 贴在标题旁边，等于告诉用户"匹配到的是哪一块"。
//
// 搜索框语法（tag: / -tag: / re:，见 search-syntax.mjs）解析出来的结果在这里被拆开用：
// 高亮只跟"正向的普通词"走，`-tag:` 之类只影响筛选，不会在界面上乱标。

import { compiledRegex, parseSearchSyntax, positiveTerms } from "./search-syntax.mjs";

// 这里特意不依赖 core/utils.js 的 escapeHtml：那个实现走 DOM（document.createElement），
// 会让这个纯函数层没法在 node --test 里跑。高亮必须自己负责转义，所以就地实现一份。
function escapeHtmlText(value) {
	return String(value ?? "")
		.replaceAll("&", "&amp;")
		.replaceAll("<", "&lt;")
		.replaceAll(">", "&gt;")
		.replaceAll('"', "&quot;")
		.replaceAll("'", "&#39;");
}

export function normalizeQuery(query) {
	return String(query ?? "").trim();
}

/**
 * subsequenceMatch 判断 query 的字符是否按顺序出现在 text 中（大小写不敏感）。
 * 与 Go 侧 fuzzyMatch 语义一致。
 */
export function subsequenceMatch(text, query) {
	const target = String(text ?? "").toLowerCase();
	const needle = String(query ?? "").toLowerCase();
	if (!needle) return true;
	let index = 0;
	for (let cursor = 0; cursor < target.length && index < needle.length; cursor += 1) {
		if (target[cursor] === needle[index]) index += 1;
	}
	return index === needle.length;
}

/**
 * findMatchRanges 返回要高亮的区间 [{ start, end, kind }]。
 * kind = "exact"（连续子串）或 "fuzzy"（逐字命中）。
 */
export function findMatchRanges(text, query) {
	const value = String(text ?? "");
	const needle = normalizeQuery(query);
	if (!value || !needle) return [];
	const lowerValue = value.toLowerCase();
	const lowerNeedle = needle.toLowerCase();

	// 1) 连续命中的全部位置（不重叠）。
	const exact = [];
	let from = 0;
	while (from <= lowerValue.length - lowerNeedle.length) {
		const index = lowerValue.indexOf(lowerNeedle, from);
		if (index < 0) break;
		exact.push({ start: index, end: index + lowerNeedle.length, kind: "exact" });
		from = index + lowerNeedle.length;
	}
	if (exact.length > 0) return exact;

	// 2) 退化成逐字命中；相邻字符合并成一段，避免每个字一个 <mark>。
	const ranges = [];
	let needleIndex = 0;
	let start = -1;
	for (let cursor = 0; cursor < lowerValue.length && needleIndex < lowerNeedle.length; cursor += 1) {
		if (lowerValue[cursor] !== lowerNeedle[needleIndex]) continue;
		if (start < 0) start = cursor;
		needleIndex += 1;
		const isLast = needleIndex === lowerNeedle.length;
		const nextMatchesCurrent = lowerValue[cursor + 1] === lowerNeedle[needleIndex];
		if (isLast || !nextMatchesCurrent) {
			ranges.push({ start, end: cursor + 1, kind: "fuzzy" });
			start = -1;
		}
	}
	return needleIndex === lowerNeedle.length ? ranges : [];
}

/** highlightMatches 生成安全的 HTML：先转义、再只把命中区间包进 <mark>。 */
/**
 * mergeMatchRanges 把多组区间合并成不重叠、升序的一段段。
 * 多个搜索词的高亮区间可能相邻或重叠（例如 "ak" 与 "ak47"），必须先合并再渲染。
 */
export function mergeMatchRanges(ranges) {
	const sorted = [...(ranges || [])].sort((left, right) => left.start - right.start || left.end - right.end);
	const merged = [];
	for (const range of sorted) {
		if (range.end <= range.start) continue;
		const last = merged[merged.length - 1];
		if (last && range.start <= last.end) {
			last.end = Math.max(last.end, range.end);
			if (range.kind === "exact") last.kind = "exact";
			continue;
		}
		merged.push({ ...range });
	}
	return merged;
}

/**
 * highlightMatches 生成安全的 HTML：先转义、再只把命中区间包进 <mark>。
 * 第二个参数支持三种写法：
 *   - 单个词："ak47"
 *   - 词数组：["ak47", "武器"]（搜索语法解析出的多个正向词）
 *   - { terms, regex }：再带上正则命中（`re:` 写法也要能看见高亮，而不是只有文字词才亮）
 */
export function highlightMatches(text, queryOrTerms = "") {
	const value = String(text ?? "");
	const ranges = mergeMatchRanges(highlightRangesFor(value, queryOrTerms));
	if (ranges.length === 0) return escapeHtmlText(value);

	let html = "";
	let cursor = 0;
	for (const range of ranges) {
		html += escapeHtmlText(value.slice(cursor, range.start));
		html += `<mark class="search-hit" data-match="${range.kind}">${escapeHtmlText(value.slice(range.start, range.end))}</mark>`;
		cursor = range.end;
	}
	html += escapeHtmlText(value.slice(cursor));
	return html;
}

/** highlightRangesFor 把三种入参形式统一成"要高亮的区间"。 */
export function highlightRangesFor(text, spec) {
	const value = String(text ?? "");
	if (typeof spec === "string" || spec == null) {
		return findMatchRanges(value, spec ?? "");
	}
	if (Array.isArray(spec)) {
		return spec.flatMap((term) => findMatchRanges(value, term));
	}

	const ranges = (spec.terms || []).flatMap((term) => findMatchRanges(value, term));
	const regex = spec.regex;
	if (regex) {
		// 逐处高亮正则命中；带 g 标志复用，零长度匹配要手动前进，避免死循环。
		const flags = regex.flags.includes("g") ? regex.flags : `${regex.flags}g`;
		const global = new RegExp(regex.source, flags);
		let match = global.exec(value);
		let guard = 0;
		while (match && guard < 64) {
			guard += 1;
			if (match[0].length > 0) {
				ranges.push({ start: match.index, end: match.index + match[0].length, kind: "regex" });
			} else {
				global.lastIndex += 1;
			}
			match = global.exec(value);
		}
	}
	return ranges;
}

// 可被搜索的字段与它们在界面上的叫法（顺序 = 展示优先级）。
const SEARCHABLE_FIELDS = [
	{ label: "标题", read: (file) => [file?.title] },
	{ label: "文件名", read: (file) => [file?.name] },
	{ label: "一级标签", read: (file) => [file?.primaryTag] },
	{ label: "子标签", read: (file) => file?.secondaryTags || [] },
	{ label: "主体", read: (file) => [file?.subjectSummary, ...(file?.contentSubjects || [])] },
	{ label: "发音角色", read: (file) => file?.voiceCharacters || [] },
	{ label: "动作槽", read: (file) => [file?.xdrSummary] },
];

/** fileTags 返回该 Mod 的全部标签（一级 + 二级）。 */
function fileTags(file) {
	const tags = [];
	const primary = String(file?.primaryTag ?? "").trim();
	if (primary) tags.push({ name: primary, label: "一级标签" });
	for (const tag of file?.secondaryTags || []) {
		const name = String(tag ?? "").trim();
		if (name) tags.push({ name, label: "子标签" });
	}
	return tags;
}

/** matchReasonLabelsFor 根据解析后的语法，算出这行是"被哪些字段匹配到"的。 */
function matchReasonLabelsFor(file, spec, max) {
	const labels = [];
	const push = (label) => {
		if (label && !labels.includes(label) && labels.length < max) labels.push(label);
	};

	// 1) 普通词：逐个找出它命中的字段。
	for (const term of positiveTerms(spec)) {
		for (const field of SEARCHABLE_FIELDS) {
			const hit = (field.read(file) || []).some((value) => {
				const text = String(value ?? "").trim();
				return text !== "" && subsequenceMatch(text, term);
			});
			// 命中几个字段就列几个：只说"标题"会让人以为文件名没命中，
			// 实际上"匹配到哪些字段"本身就是用户想知道的信息（上限由 max 控制）。
			if (hit) push(field.label);
		}
	}

	// 2) 正则：命中哪些字段就标哪些。
	const regex = compiledRegex(spec);
	if (regex) {
		for (const field of SEARCHABLE_FIELDS) {
			const hit = (field.read(file) || []).some((value) => {
				const text = String(value ?? "").trim();
				return text !== "" && regex.test(text);
			});
			if (hit) push(field.label);
		}
	}

	// 3) tag: 条件：标成"一级标签 / 子标签"，说明这是标签层面的命中。
	const tags = fileTags(file);
	for (const condition of spec?.includeTags || []) {
		const matched = tags.find((tag) =>
			condition.some((name) => tag.name.toLowerCase() === String(name).toLowerCase()),
		);
		if (matched) push(matched.label);
	}
	return labels;
}

/**
 * describeMatchReasons 返回命中的字段名（最多 max 个）。
 * 用于"明确的匹配"：让用户知道是标题命中了，还是某个标签/主体命中了。
 * 支持搜索框语法：`tag:` 命中会标成"一级标签/子标签"，`-词` 与 `re:` 不会乱标高亮。
 */
export function describeMatchReasons(file, query, max = 3) {
	const spec = parseSearchSyntax(query);
	if (!spec.raw) return [];
	return matchReasonLabelsFor(file, spec, max);
}

/** formatMatchReasonChip 把命中字段拼成 chip 文案。 */
export function formatMatchReasonChip(labels) {
	const list = (labels || []).filter(Boolean);
	if (list.length === 0) return "";
	return `匹配：${list.join(" · ")}`;
}

/** describeSearchResult 生成搜索框旁边的计数文案。 */
export function describeSearchResult({ total = 0, shown = 0, query = "", regexInvalid = "" } = {}) {
	const keyword = normalizeQuery(query);
	if (!keyword) return "";
	if (regexInvalid) return `正则表达式无效：${regexInvalid}`;
	if (shown === 0) return `没有匹配「${keyword}」的 Mod（共 ${Number(total) || 0} 个）`;
	return `匹配 ${Number(shown) || 0} / ${Number(total) || 0} 个 Mod`;
}
