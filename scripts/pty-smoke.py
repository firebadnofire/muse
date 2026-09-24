#!/usr/bin/env python3
"""Optional Linux PTY E2E check. Requires pexpect; uses a local fake HTTP backend.
Does not execute generated source. Runs ordinary test commands and Vim/htop.
Run after building: python3 scripts/pty-smoke.py ./bin/muse
"""
import http.server
import json
import os
from pathlib import Path
import shutil
import sys
import tempfile
import threading
import time
import termios
import pexpect

test_shell = os.getenv('MUSE_TEST_SHELL', 'bash')
binary = str(Path(sys.argv[1] if len(sys.argv) > 1 else './bin/muse').resolve())
with tempfile.TemporaryDirectory(prefix='muse-e2e-') as tmp:
    root = Path(tmp)
    marker = root / 'MUST_NOT_EXIST'
    suggestion = f"touch '{marker}'; echo $(printf text); printf '%s\\n' 'a\\b'"

    class Backend(http.server.BaseHTTPRequestHandler):
        def log_message(self, *args): pass
        def do_GET(self):
            self.send_response(200); self.end_headers()
            self.wfile.write(json.dumps({'models': [{'name': 'qwen2.5-coder:7b'}, {'name': 'fixture:exact-tag'}]}).encode())
        def do_POST(self):
            request = json.loads(self.rfile.read(int(self.headers['Content-Length'])))
            self.send_response(200); self.end_headers()
            text = suggestion
            if request['messages'][1]['content'] == 'invalid-generation': text = 'echo first\necho second'
            if 'multiline' in request['messages'][0]['content']:
                text += '\nprintf second\n'
            for part in [text[:8], text[8:]]:
                self.wfile.write((json.dumps({'message': {'content': part}})+'\n').encode()); self.wfile.flush(); time.sleep(.03)
            self.wfile.write(b'{"done":true}\n'); self.wfile.flush()
    server = http.server.ThreadingHTTPServer(('127.0.0.1', 0), Backend)
    threading.Thread(target=server.serve_forever, daemon=True).start()
    env = os.environ.copy()
    for name in list(env):
        if name.startswith('MUSE_'): del env[name]
    env.update(TMPDIR=tmp, ZDOTDIR=tmp, HOME=tmp, XDG_CONFIG_HOME=str(root/'config'), XDG_DATA_HOME=str(root/'data'),
               TERM='xterm-256color', SHELL='/bin/'+test_shell, EDITOR='vi -u NONE -n',
               MUSE_ENDPOINT=f'http://127.0.0.1:{server.server_port}')
    (root/('.bashrc' if test_shell=='bash' else '.zshrc')).write_text("PS1='MUSE_TEST> '\n")
    if test_shell == 'zsh': (root/'.zshenv').write_text('unsetopt globalrcs\n')
    child = pexpect.spawn(binary, ['--shell', '/bin/'+test_shell], env=env, encoding='utf-8', timeout=20, dimensions=(30, 100))
    if os.getenv('MUSE_TEST_LOG'): child.logfile = sys.stdout
    try:
        child.expect('MUSE_TEST> ')
        child.sendline('stty size'); child.expect('30 100'); child.expect('MUSE_TEST> ')
        child.setwinsize(36, 110); time.sleep(.1)
        child.sendline('stty size'); child.expect('36 110'); child.expect('MUSE_TEST> ')
        child.sendline('sleep 30'); time.sleep(.2); child.sendcontrol('c'); child.expect('MUSE_TEST> ')
        if shutil.which('vim'):
            child.sendline('vim -u NONE -n'); time.sleep(.5); child.send(':q!\r'); child.expect('MUSE_TEST> ')
        if shutil.which('htop'):
            child.sendline('htop'); time.sleep(.5); child.send('q'); child.expect('MUSE_TEST> ')
        child.sendline('eval "$(muse integration '+test_shell+')"'); child.expect('MUSE_TEST> ')
        capture = root/'buffer'
        if test_shell == "bash":
            child.sendline("_capture() { printf %s \"$READLINE_LINE\" > '"+str(capture)+"'; }; bind -x '\"\\C-xv\":_capture'")
        else:
            child.sendline("_capture() { print -rn -- \"$BUFFER\" > "+str(capture)+"; }; zle -N _capture; bindkey '^Xv' _capture")
        child.expect('MUSE_TEST> ')
        if test_shell == 'zsh':
            child.sendline(binary+" generate 'test request'"); child.expect('MUSE_TEST> ')
            child.send('\x18v'); time.sleep(.2)
            assert capture.read_text() == suggestion, 'generate did not stage exact text'
            assert not marker.exists(), 'generate executed the suggestion'
            child.sendcontrol('c'); child.expect('MUSE_TEST> ')
            child.sendline(binary+" generate --print 'test request'"); child.expect('MUSE_TEST> ')
            child.send('\x18v'); time.sleep(.2)
            assert capture.read_text() == '', '--print unexpectedly staged text'
            child.sendline(binary+" generate 'invalid-generation'"); child.expect('MUSE_TEST> ')
            child.send('\x18v'); time.sleep(.2)
            assert capture.read_text() == '', 'invalid generation was staged'
            assert not marker.exists()
        if test_shell == 'zsh': child.sendline(binary+' composer')
        else: child.send('\x18g')
        child.expect('Connected')
        child.send('generate a test command\r'); child.expect('Review suggestion')
        child.send('\x01'); time.sleep(.5); child.send('\x18v'); time.sleep(.2)
        assert capture.read_text() == suggestion, capture.read_text()
        assert not marker.exists(), 'acceptance executed a command'
        # Existing text must remain intact and must not reopen the composer.
        child.send('\x18g'); child.expect('existing input preserved')
        child.send('\x18v'); time.sleep(.2); assert capture.read_text() == suggestion
        child.sendcontrol('c'); child.expect('MUSE_TEST> ')
        if test_shell == 'zsh': child.sendline(binary+' composer')
        else: child.send('\x18g')
        child.expect('Connected')
        child.send('\x14')  # Compose mode
        child.send('generate a script\r'); time.sleep(1)
        # Real editor handoff: append a comment, save, return to composer review.
        child.send('Go# reviewed by PTY smoke\x1b:wq\r'); child.expect('Edited draft')
        child.send('\x01'); time.sleep(.5); child.send('\x18v'); time.sleep(.2)
        invocation = capture.read_text()
        assert 'reviewed-' in invocation and '\n' not in invocation, invocation
        files = list((root/'data'/'muse'/'scripts').glob('reviewed-*.sh'))
        assert len(files)==1 and '# reviewed by PTY smoke' in files[0].read_text()
        assert files[0].stat().st_mode & 0o777 == 0o600
        assert not marker.exists(), 'Compose acceptance executed a script'
        child.sendcontrol('c'); child.expect('MUSE_TEST> ')
        # Change model interactively; persisted exact tag must be honored on reopening.
        child.send('\x18g'); child.expect('Connected'); child.send('\x0c'); child.expect('Installed models')
        child.send('\x1b[B\r'); time.sleep(.3); child.sendcontrol('c'); child.expect('MUSE_TEST> ')
        assert 'fixture:exact-tag' in (root/'config'/'muse'/'config.toml').read_text()
        child.send('\x18g'); child.expect('fixture:exact-tag'); child.sendcontrol('c'); child.expect('MUSE_TEST> ')
        child.sendline('MUSE_ENDPOINT=http://127.0.0.1:1 muse composer --manual'); child.expect('server unavailable')
        child.sendcontrol('c'); child.expect('MUSE_TEST> ')
        child.sendline('true'); child.expect('MUSE_TEST> ')
        child.sendcontrol('d'); child.expect(pexpect.EOF)
        attrs=termios.tcgetattr(child.child_fd)
        assert attrs[3] & termios.ICANON and attrs[3] & termios.ECHO, 'terminal modes not restored'
        child.close()
        assert child.exitstatus == 0, child.exitstatus
        assert not list(root.glob("muse-stage.*")), "session directory not cleaned"
        print('PASS: resize, Ctrl-C/D, Vim, htop, native staging, existing input, Compose editor/file, model persistence, clean exit; generated text never executed.')
    except Exception:
        print('PTY output tail:', repr(child.before), file=sys.stderr)
        raise
    finally:
        child.close(force=True); server.shutdown(); server.server_close()
