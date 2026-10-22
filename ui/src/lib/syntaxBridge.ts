import './monacoEnv';
import * as monaco from 'monaco-editor/editor/editor.api.js';
import { backend } from './api';
import type { DocumentView, SyntaxToken } from './types';

const juliaTokenTypes = [
  'identifier', 'keyword', 'operator', 'number', 'boolean', 'string', 'stringDelimiter',
  'stringPrefix', 'stringEscape', 'command', 'character', 'comment', 'interpolation',
  'macro', 'punctuation', 'error'
];
const tokenTypeIndex = new Map(juliaTokenTypes.map((kind, index) => [kind, index]));
const refreshListeners = new Set<(event: void) => void>();
const decorationIds = new WeakMap<monaco.editor.ITextModel, string[]>();
const utf8Encoder = new TextEncoder();
const utf8Decoder = new TextDecoder();
let juliaReady = false;

interface TokenSpan {
  line: number;
  start: number;
  length: number;
  type: number;
  kind: string;
}

export function createJuliaEditor(
  element: HTMLElement,
  onChange: (path: string, text: string) => void,
  onSave: (path: string, text: string) => void
): monaco.editor.IStandaloneCodeEditor {
  registerJuliaLanguage();

  const editor = monaco.editor.create(element, {
    value: '',
    language: 'julia',
    theme: 'jide-dark',
    automaticLayout: true,
    fontFamily: 'JetBrains Mono, Cascadia Code, Consolas, monospace',
    fontSize: 13,
    lineHeight: 22,
    minimap: { enabled: false },
    padding: { top: 16, bottom: 20 },
    scrollBeyondLastLine: false,
    renderLineHighlight: 'line',
    roundedSelection: false,
    wordWrap: 'off',
    tabSize: 4,
    insertSpaces: true,
    bracketPairColorization: { enabled: true },
    guides: { indentation: true, bracketPairs: true },
    overviewRulerBorder: false,
    hideCursorInOverviewRuler: true,
    smoothScrolling: true,
    cursorSmoothCaretAnimation: 'on'
  });

  editor.onDidChangeModelContent(() => {
    const model = editor.getModel();
    if (model) onChange(model.uri.fsPath, model.getValue());
  });
  editor.addCommand(monaco.KeyMod.CtrlCmd | monaco.KeyCode.KeyS, () => {
    const model = editor.getModel();
    if (model) onSave(model.uri.fsPath, model.getValue());
  });
  return editor;
}

export function updateJuliaHighlights(): void {
  for (const listener of refreshListeners) listener(undefined);
}

export function applyDocument(editor: monaco.editor.IStandaloneCodeEditor, document: DocumentView): void {
  const uri = monaco.Uri.file(document.path);
  let model = monaco.editor.getModel(uri);
  if (!model) {
    model = monaco.editor.createModel(document.text, languageForPath(document.path), uri);
  } else if (model.getValue() !== document.text) {
    model.setValue(document.text);
  }
  if (model.getLanguageId() !== languageForPath(document.path)) {
    monaco.editor.setModelLanguage(model, languageForPath(document.path));
  }
  if (editor.getModel() !== model) editor.setModel(model);
  editor.updateOptions({ readOnly: false });
  updateJuliaHighlights();
}

export function removeDocumentModel(path: string): void {
  const model = monaco.editor.getModel(monaco.Uri.file(path));
  if (model) model.dispose();
}

export function disposeEditor(editor: monaco.editor.IStandaloneCodeEditor): void {
  editor.dispose();
}

function languageForPath(path: string): string {
  const extension = path.split('.').at(-1)?.toLowerCase();
  if (extension === 'jl') return 'julia';
  if (extension === 'toml') return 'toml';
  if (extension === 'md' || extension === 'markdown') return 'markdown';
  if (extension === 'json') return 'json';
  if (extension === 'yaml' || extension === 'yml') return 'yaml';
  return 'plaintext';
}

function registerJuliaLanguage(): void {
  if (juliaReady) return;
  juliaReady = true;
  monaco.languages.register({ id: 'julia' });
  monaco.languages.setLanguageConfiguration('julia', {
    comments: { lineComment: '#', blockComment: ['#=', '=#'] },
    brackets: [['(', ')'], ['[', ']'], ['{', '}']],
    autoClosingPairs: [
      { open: '(', close: ')' }, { open: '[', close: ']' }, { open: '{', close: '}' },
      { open: '"', close: '"', notIn: ['string', 'comment'] },
      { open: '`', close: '`', notIn: ['string', 'comment'] }
    ],
    surroundingPairs: [
      { open: '(', close: ')' }, { open: '[', close: ']' }, { open: '{', close: '}' },
      { open: '"', close: '"' }, { open: '`', close: '`' }
    ]
  });
  monaco.languages.setMonarchTokensProvider('julia', juliaMonarch);
  monaco.editor.defineTheme('jide-dark', {
    base: 'vs-dark',
    inherit: true,
    rules: [
      { token: 'keyword', foreground: 'c792ea' }, { token: 'number', foreground: 'f78c6c' },
      { token: 'string', foreground: 'c3e88d' }, { token: 'comment', foreground: '697386', fontStyle: 'italic' },
      { token: 'identifier', foreground: 'd7deea' }, { token: 'operator', foreground: '89ddff' }
    ],
    colors: {
      'editor.background': '#0b1018',
      'editor.foreground': '#d7deea',
      'editorLineNumber.foreground': '#4f5c6d',
      'editorLineNumber.activeForeground': '#91a0b2',
      'editor.lineHighlightBackground': '#111a25',
      'editor.selectionBackground': '#254238',
      'editorCursor.foreground': '#72e0c2'
    }
  });

  monaco.languages.registerDocumentSemanticTokensProvider('julia', {
    getLegend: () => ({ tokenTypes: juliaTokenTypes, tokenModifiers: [] }),
    onDidChange: (listener) => {
      refreshListeners.add(listener);
      return { dispose: () => refreshListeners.delete(listener) };
    },
    provideDocumentSemanticTokens: async (model, _lastResultId, cancellationToken) => {
      try {
        const modelVersion = model.getVersionId();
        const result = await backend.syntaxHighlights(model.uri.toString(), model.getValue());
        if (cancellationToken.isCancellationRequested || model.getVersionId() !== modelVersion) {
          return { data: new Uint32Array() };
        }
        const spans = buildTokenSpans(model, result.tokens);
        const data = encodeSemanticTokens(spans);
        const oldDecorations = decorationIds.get(model) ?? [];
        const newDecorations = model.deltaDecorations(oldDecorations, spans.map((span) => ({
          range: new monaco.Range(span.line + 1, span.start + 1, span.line + 1, span.start + span.length + 1),
          options: {
            inlineClassName: `jide-syntax-token jide-token-${span.kind}`,
            stickiness: monaco.editor.TrackedRangeStickiness.NeverGrowsWhenTypingAtEdges
          }
        })));
        decorationIds.set(model, newDecorations);
        const markers = result.diagnostics.map((diagnostic) => {
          const startLine = diagnostic.range.start.line + 1;
          const endLine = diagnostic.range.end.line + 1;
          const startText = model.getLineContent(startLine);
          const endText = model.getLineContent(endLine);
          return {
            severity: monaco.MarkerSeverity.Error,
            message: diagnostic.message,
            source: 'Tree-sitter Julia',
            startLineNumber: startLine,
            startColumn: byteColumnToUTF16(startText, diagnostic.range.start.column) + 1,
            endLineNumber: endLine,
            endColumn: Math.max(
              byteColumnToUTF16(endText, diagnostic.range.end.column) + 1,
              byteColumnToUTF16(startText, diagnostic.range.start.column) + 2
            )
          };
        });
        monaco.editor.setModelMarkers(model, 'jide-syntax', markers);
        return { data, resultId: String(model.getVersionId()) };
      } catch {
        return { data: new Uint32Array() };
      }
    },
    releaseDocumentSemanticTokens: () => undefined
  });
}

function buildTokenSpans(model: monaco.editor.ITextModel, tokens: SyntaxToken[]): TokenSpan[] {
  const spans: TokenSpan[] = [];
  for (const token of tokens) {
    const type = tokenTypeIndex.get(token.kind);
    if (type === undefined) continue;
    const firstLine = Math.max(0, token.range.start.line);
    const lastLine = Math.min(model.getLineCount() - 1, token.range.end.line);
    for (let line = firstLine; line <= lastLine; line += 1) {
      const lineText = model.getLineContent(line + 1);
      const startByte = line === firstLine ? token.range.start.column : 0;
      const endByte = line === lastLine ? token.range.end.column : utf8Encoder.encode(lineText).length;
      const start = byteColumnToUTF16(lineText, startByte);
      const end = byteColumnToUTF16(lineText, endByte);
      if (end > start) spans.push({ line, start, length: end - start, type, kind: token.kind });
    }
  }
  spans.sort((left, right) => left.line - right.line || left.start - right.start);
  return spans;
}

function encodeSemanticTokens(spans: TokenSpan[]): Uint32Array {
  const encoded: number[] = [];
  let previousLine = 0;
  let previousStart = 0;
  for (let index = 0; index < spans.length; index += 1) {
    const span = spans[index];
    const deltaLine = span.line - previousLine;
    const deltaStart = deltaLine === 0 ? span.start - previousStart : span.start;
    encoded.push(deltaLine, deltaStart, span.length, span.type, 0);
    previousLine = span.line;
    previousStart = span.start;
  }
  return new Uint32Array(encoded);
}

function byteColumnToUTF16(line: string, byteColumn: number): number {
  const bytes = utf8Encoder.encode(line);
  const prefix = bytes.subarray(0, Math.max(0, Math.min(byteColumn, bytes.length)));
  return utf8Decoder.decode(prefix).length;
}

const juliaMonarch: monaco.languages.IMonarchLanguage = {
  defaultToken: 'identifier',
  tokenPostfix: '.jl',
  keywords: [
    'abstract', 'baremodule', 'begin', 'break', 'catch', 'const', 'continue', 'do', 'else',
    'elseif', 'end', 'export', 'finally', 'for', 'function', 'global', 'if', 'import', 'let',
    'local', 'macro', 'module', 'mutable', 'primitive', 'public', 'quote', 'return', 'struct',
    'try', 'using', 'while', 'where', 'in', 'isa', 'true', 'false'
  ],
  operators: [
    '=', '>', '<', '!', '~', '?', ':', '==', '<=', '>=', '!=', '&&', '||', '=>', '->', '+',
    '-', '*', '/', '^', '%', '+=', '-=', '*=', '/=', '|>', '.|>'
  ],
  tokenizer: {
    root: [
      [/#=/, 'comment', '@blockComment'],
      [/#.*$/, 'comment'],
      [/r"""/, 'string', '@tripleString'],
      [/"""/, 'string', '@tripleString'],
      [/r"/, 'string', '@rawString'],
      [/"/, 'string', '@string'],
      [/`/, 'string', '@command'],
      [/'(?:[^'\\]|\\.)'/, 'string'],
      [/@[A-Za-z_][\w!]*/, 'type.identifier'],
      [/[A-Za-z_][\w!]*(?=\s*\()/, 'function'],
      [/[A-Za-z_][\w!]*/, { cases: { '@keywords': 'keyword', '@default': 'identifier' } }],
      [/[0-9]+(?:\.[0-9]+)?(?:[eE][+-]?[0-9]+)?/, 'number'],
      [/[+\-*/^%=<>!&|~:.?]+/, { cases: { '@operators': 'operator', '@default': 'operator' } }],
      [/[(){}\[\],;]/, 'delimiter'],
      [/\s+/, 'white']
    ],
    blockComment: [[/=#/, 'comment', '@pop'], [/[^#=]+/, 'comment'], [/[#=]/, 'comment']],
    string: [
      [/\\./, 'string.escape'],
      [/\$\(/, 'delimiter.bracket', '@interpolation'],
      [/\$[A-Za-z_][\w!]*/, 'variable'],
      [/"/, 'string', '@pop'],
      [/[^\\"$]+/, 'string'],
      [/\$/, 'string']
    ],
    rawString: [[/"/, 'string', '@pop'], [/[^\"]+/, 'string']],
    tripleString: [
      [/\\./, 'string.escape'],
      [/\$\(/, 'delimiter.bracket', '@interpolation'],
      [/\$[A-Za-z_][\w!]*/, 'variable'],
      [/"""/, 'string', '@pop'],
      [/[^\\"$]+/, 'string'],
      [/./, 'string']
    ],
    command: [
      [/\\./, 'string.escape'],
      [/\$\(/, 'delimiter.bracket', '@interpolation'],
      [/\$[A-Za-z_][\w!]*/, 'variable'],
      [/`/, 'string', '@pop'],
      [/[^\\`$]+/, 'string'],
      [/\$/, 'string']
    ],
    interpolation: [
      [/\)/, 'delimiter.bracket', '@pop'],
      [/\(/, 'delimiter.bracket', '@push'],
      { include: 'root' }
    ]
  }
};
