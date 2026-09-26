// 搜索框语法的前端镜像（与 Go 侧 internal/app/search_query.go 同语义）。
//
// 为什么要在前端再实现一遍：过滤由 Go 的 SearchVPKFiles 负责（权威），
// 但"高亮哪几个词"和"为什么这行被匹配到"必须现算，不能为了这两件小事再跑一次后端。
// 两边共用同一份用例（internal/app/testdata/search_query_cases.json），测试会同时校验，
// 避免实现漂移。
//
// 语法（对齐 FireAxe AddonNodeSearchUtils 的 IsRegex / Tags / TagFilterMode）：
//   ak47            普通词：模糊匹配（字符按顺序出现）
//   "ak 47"         引号里算一个词
//   tag:步枪|狙击     命中任一标签（= FireAxe TagFilterMode.Or）
//   tag:武器 tag:步枪 两个标签都要（= And）
//   -tag:材质        排除含该标签的（= Not）
//   -材质            普通词的排除写法
//   re:^ak\d+$      正则匹配（= IsRegex）

export const TAG_PREFIXES = ["tag:", "标签:"];
export const EXCLUDE_TAG_PREFIXES = ["-tag:", "-标签:"];
export const REGEX_PREFIXES = ["re:", "正则:"];

function startsWithAny(value, prefixes) {
	return prefixes.some((prefix) => value.startsWith(prefix));
}

function stripAnyPrefix(value, prefixes) {
	for (const prefix of prefixes) {
		if (value.startsWith(prefix)) return value.slice(prefix.length).trim();
	}
	return value.trim();
}

/** tokenizeSearchQuery 按空白切词，双引号内的空格不算分隔符。 */
export function tokenizeSearchQuery(raw) {
	const tokens = [];
	let current = "";
	let inQuotes = false;
	for (const char of String(raw ?? "")) {
		if (char === '"') {
			inQuotes = !inQuotes;
			continue;
		}
		if (!inQuotes && /\s/.test(char)) {
			const token = current.trim();
			if (token) tokens.push(token);
			current = "";
			continue;
		}
		current += char;
	}
	const tail = current.trim();
	if (tail) tokens.push(tail);
	return tokens;
}

/** splitTagAlternatives 把 `a|b` 拆成 OR 组（去空、去重、保序）。 */
export function splitTagAlternatives(value) {
	const result = [];
	const seen = new Set();
	for (const part of String(value ?? "").split("|")) {
		const name = part.trim();
		if (!name) continue;
		const key = name.toLowerCase();
		if (seen.has(key)) continue;
		seen.add(key);
		result.push(name);
	}
	return result;
}

/**
 * parseSearchSyntax 解析搜索框内容。
 * 返回 { raw, terms, includeTags, excludeTags, regex, regexInvalid }：
 *   - terms：字符串数组，前导 "-" 表示"命中即排除"；
 *   - includeTags：数组的数组，内层是"任一命中即可"的 OR 组；
 *   - regex：正则源码（不带 (?i)，匹配时用 i 标志）；
 *   - regexInvalid：正则写错时的错误信息（此时界面应显示原因，且不渲染任何命中）。
 */
export function parseSearchSyntax(raw) {
	const spec = {
		raw: String(raw ?? "").trim(),
		terms: [],
		includeTags: [],
		excludeTags: [],
		regex: "",
		regexInvalid: "",
	};

	for (const token of tokenizeSearchQuery(spec.raw)) {
		if (startsWithAny(token, REGEX_PREFIXES)) {
			const pattern = stripAnyPrefix(token, REGEX_PREFIXES);
			if (!pattern) continue;
			try {
				new RegExp(pattern, "i");
				spec.regex = pattern;
			} catch (error) {
				spec.regexInvalid = String(error?.message || error);
			}
			continue;
		}
		if (startsWithAny(token, EXCLUDE_TAG_PREFIXES)) {
			const name = stripAnyPrefix(token, EXCLUDE_TAG_PREFIXES);
			if (!name) continue;
			spec.excludeTags.push(...splitTagAlternatives(name));
			continue;
		}
		if (startsWithAny(token, TAG_PREFIXES)) {
			const name = stripAnyPrefix(token, TAG_PREFIXES);
			if (!name) continue;
			const alternatives = splitTagAlternatives(name);
			if (alternatives.length > 0) spec.includeTags.push(alternatives);
			continue;
		}
		spec.terms.push(token);
	}
	return spec;
}

/** positiveTerms 返回需要高亮的词（排除 `-词`）。 */
export function positiveTerms(spec) {
	return (spec?.terms || []).filter((term) => !term.startsWith("-")).map((term) => term);
}

/** negatedTerms 返回 `-词` 里的词。 */
export function negatedTerms(spec) {
	return (spec?.terms || [])
		.filter((term) => term.startsWith("-") && term.length > 1)
		.map((term) => term.slice(1));
}

/** compiledRegex 返回编译好的正则；没有或非法时返回 null。 */
export function compiledRegex(spec) {
	if (!spec?.regex || spec.regexInvalid) return null;
	try {
		return new RegExp(spec.regex, "i");
	} catch (error) {
		return null;
	}
}
