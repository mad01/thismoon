"""Regenerate python.json, the golden table pyjson's tests compare against.

Run from this directory with the Python whose json module is the reference:
    python3 -I golden.py > python.json
Every expected value comes from Python itself: json.dumps output, the
JSONDecodeError text, and the result of == between two parsed documents.
"""

import json

VALID = [
    '{}',
    '[]',
    '{"a": 1, "b": [1, 2, {"c": null}], "d": {}}',
    '{"b": 1, "a": 2, "b": 3}',
    '[1.0, 1e16, 1e15, 0.0001, 0.00001, -0.0, 1.5e-7, 123456789.123, 1E400, -1e400,'
    ' 5e-324, 1.7976931348623157e308, 0.1, 2e16, 100.5e-2, 1e-400, 3.14159, 1e100, 12e-1]',
    '[NaN, Infinity, -Infinity]',
    '[-0, 123456789012345678901234567890, -5, 0]',
    '["caf\u00e9", "\\u00e9", "\\ud83d\\ude00", "\U0001F600", "\\ud800", "\\udc00x",'
    ' "\\ud800\\ud800", "a/b<>&", "\\u007f\\u0001\\b\\f\\n\\r\\t\\"\\\\", "\\/"]',
    ' \t\n {"nested": {"deep": [[], [{}], {"x": [true, false]}]}} \n',
    '{"\u00e9": 1, "\\ud800": 2, "": 3}',
    '{"z": {"b": 1, "a": [{"d": 1, "c": 2}]}, "y": null}',
]

INVALID = [
    '', ' ', '{', '{"a"', '{"a":', '{"a": 1', '{"a": 1,}', '[1,]', '[1 2]', '{"a" 1}',
    '{1: 2}', '"abc', '"a\x01b"', '"\\x"', '"\\u12"', '"\\u12zz"', '"\\ud800\\u12zz"',
    '01', '1.', '1e', '1e+', '-', '-x', 'nul', 'tru', 'fals', 'Na', '{"a": 1} x',
    '\ufeff{}', '[1,\n 2,\n ]', '{"a":\n\n  }', '{,}', '[,]', 'Infinit', '-Infinit',
    '"\u00e9\u00e9" x', '"abc\\', '[1, 2', '{"a": [}',
]

# Pairs compared with Python's ==, the rule the merge and the diff use.
EQUAL = [
    ('1', '1.0'), ('true', '1'), ('false', '0'), ('true', '1.0'), ('1', '1.5'),
    ('9007199254740993', '9007199254740992.0'), ('[1, 2]', '[1.0, 2]'),
    ('{"a": 1, "b": 2}', '{"b": 2, "a": 1}'), ('{"a": 1}', '{"a": 1, "b": 2}'),
    ('[1, 2]', '[2, 1]'), ('null', 'false'), ('"1"', '1'), ('[]', '{}'),
    ('{"a": [true]}', '{"a": [1]}'), ('1e400', '1e400'), ('-0.0', '0'),
    ('"\\u00e9"', '"\u00e9"'), ('[null]', '[null]'), ('{}', '{}'),
]


def main():
    valid = []
    for text in VALID:
        value = json.loads(text)
        valid.append({
            "in": text,
            "compact": json.dumps(value),
            "indent": json.dumps(value, indent=2),
            "indent0": json.dumps(value, indent=0),
            "sorted": json.dumps(value, sort_keys=True),
        })
    invalid = []
    for text in INVALID:
        try:
            json.loads(text)
        except json.JSONDecodeError as exc:
            invalid.append({"in": text, "err": str(exc)})
        else:
            raise SystemExit(f"expected a decode error for {text!r}")
    equal = [
        {"a": a, "b": b, "equal": json.loads(a) == json.loads(b)} for a, b in EQUAL
    ]
    print(json.dumps({"valid": valid, "invalid": invalid, "equal": equal}, indent=2))


main()
