# Translating

wthr ships in several languages. Any language can be added or improved without
reading the code: a translation is one JSON file.

`src/common/i18n/locales/en.json` is the single source. Translators never edit
it; they translate it.

## Supported languages

| Code | Language | Direction |
|------|----------|-----------|
| `en` | English | ltr |
| `es` | Spanish | ltr |
| `fr` | French | ltr |
| `de` | German | ltr |
| `it` | Italian | ltr |
| `ar` | Arabic | rtl |
| `zh` | Chinese | ltr |
| `ja` | Japanese | ltr |

The first six are the required minimum. Chinese and Japanese are extra languages
the project chose to keep. The count above always equals the number of files in
`src/common/i18n/locales/`.

## Who translates

AI first, humans refine. An AI assistant writes the translation for every
language it is fluent in, and a human translator corrects anything that reads
wrong. Anyone can also translate or fix a language by hand: open a pull request
with ONE locale file `{lang}.json`. Updating an existing language is the same:
edit that language's file only. Human corrections win over AI wording on every
later sync.

## File format

- UTF-8 JSON, same keys and key order as `en.json`, 2-space indent
- No trailing commas, no comments
- Keys are never translated or renamed; only values are
- The `meta` block holds `language`, `name`, `native_name`, `direction` (`ltr` or
  `rtl`), `version`, and optionally `translators`

## Placeholders

`{count}`, `{total}`, `{name}` and every other `{name}` stays exactly as in
English (same name, same braces); only its position in the sentence may move.
A value with a missing or renamed placeholder fails the build.

## Plurals

Provide every CLDR plural category the language needs (`zero`, `one`, `two`,
`few`, `many`, `other`). `other` is always required. A missing or extra category
fails the build.

| Language | Categories |
|----------|------------|
| English, Spanish, French, German | `one`, `other` |
| Italian | `one`, `many`, `other` |
| Arabic | `zero`, `one`, `two`, `few`, `many`, `other` |
| Chinese, Japanese | `other` |

## Style

- Each value is a complete phrase; never rely on joining fragments
- One consistent register per language (formal "you": `Sie`, `usted`, `vous`, `Lei`)
- Short and neutral, matching the English tone; no slang or idioms
- Follow the language's own typography (French non-breaking space before `: ; ! ?`)
- Numbers, dates and currency come from `golang.org/x/text`, not from the translation

## What stays untranslated

Product and brand names, protocol and technology names (API, Tor, `.onion`, I2P,
JSON, token), code, file names, config keys, URLs and literal values users type
(`dark`, `auto`, `metric`, `imperial`).

## Glossary

| English | Use the same term every time |
|---------|------------------------------|
| forecast | the multi-day outlook |
| alert | a severe weather warning |
| passkey | WebAuthn credential |
| token | API token (left untranslated) |
| saved location | a place a signed-in user stored |

## Layout check

Translations are often up to 40% longer than English. Open the app in the
language at 320, 768 and 1280 px and confirm nothing is clipped or overflowing.
Right-to-left languages are also checked mirrored.

## Adding a language

1. Copy `src/common/i18n/locales/en.json` to `src/common/i18n/locales/{lang}.json`
2. Translate every value and fill the `meta` block
3. Add the language to the registry and to `server.i18n.available_languages`
4. Add the row to the table above and update the language list in `README.md`
5. Run `make test` (`TestKeyConsistency` and `TestLocalesFS` check every language against `en.json`)

## Review

A maintainer checks that tests pass, placeholders and plurals are intact, the
glossary is respected, the layout check was done, and the README count and the
credits below are updated. Native-speaker review is welcome but not required to
merge.

## Credits

Translators are credited here and in the `meta.translators` field of their
locale file (their choice of name or handle).

- Initial translations: AI-assisted, awaiting native-speaker review
