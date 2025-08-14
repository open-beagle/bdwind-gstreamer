/**
 * InputManager - 输入处理管理器
 * 统一管理键盘、鼠标、触摸和游戏手柄输入
 */
class InputManager {
    constructor(eventBus, config) {
        this.eventBus = eventBus;
        this.config = config;
        
        // 输入状态
        this.isEnabled = false;
        this.inputFocused = false;
        this.currentMode = 'desktop'; // desktop, mobile, gamepad
        
        // 输入元素
        this.targetElement = null;
        this.videoContainer = null;
        
        // 输入处理器
        this.keyboardHandler = null;
        this.mouseHandler = null;
        this.touchHandler = null;
        this.gamepadHandler = null;
        this.shortcutHandler = null;
        
        // 输入统计
        this.inputStats = {
            keyboardEvents: 0,
            mouseEvents: 0,
            touchEvents: 0,
            gamepadEvents: 0,
            totalEvents: 0,
            lastEventTime: null
        };
        
        // 配置
        this.enableKeyboard = true;
        this.enableMouse = true;
        this.enableTouch = true;
        this.enableGamepad = false;
        this.mouseSensitivity = 1.0;
        this.keyboardLayout = 'qwerty';
        
        this._loadConfig();
        this._initializeHandlers();
    }

    /**
     * 加载配置
     * @private
     */
    _loadConfig() {
        if (this.config) {
            const inputConfig = this.config.get('input', {});
            this.enableKeyboard = inputConfig.enableKeyboard !== false;
            this.enableMouse = inputConfig.enableMouse !== false;
            this.enableTouch = inputConfig.enableTouch !== false;
            this.enableGamepad = inputConfig.enableGamepad === true;
            this.mouseSensitivity = inputConfig.mouseSensitivity || 1.0;
            this.keyboardLayout = inputConfig.keyboardLayout || 'qwerty';
        }
    }

    /**
     * 初始化输入处理器
     * @private
     */
    _initializeHandlers() {
        // 键盘处理器
        this.keyboardHandler = new KeyboardHandler(this.eventBus, this.config);
        
        // 鼠标处理器
        this.mouseHandler = new MouseHandler(this.eventBus, this.config);
        
        // 触摸处理器
        this.touchHandler = new TouchHandler(this.eventBus, this.config);
        
        // 游戏手柄处理器
        this.gamepadHandler = new GamepadHandler(this.eventBus, this.config);
        
        // 快捷键处理器
        this.shortcutHandler = new ShortcutHandler(this.eventBus, this.config);
    }

    /**
     * 设置目标元素
     */
    setTargetElement(element, videoContainer = null) {
        this.targetElement = element;
        this.videoContainer = videoContainer;
        
        // 更新处理器的目标元素
        this.keyboardHandler.setTargetElement(element);
        this.mouseHandler.setTargetElement(element);
        this.touchHandler.setTargetElement(element);
        this.shortcutHandler.setTargetElement(element);
        
        Logger.info('InputManager: 目标元素已设置');
    }

    /**
     * 启用输入处理
     */
    enable() {
        if (this.isEnabled) {
            Logger.warn('InputManager: 输入处理已启用');
            return;
        }
        
        Logger.info('InputManager: 启用输入处理');
        
        // 检测设备类型并设置模式
        this._detectInputMode();
        
        // 启用相应的输入处理器
        if (this.enableKeyboard) {
            this.keyboardHandler.enable();
        }
        
        if (this.enableMouse && (this.currentMode === 'desktop' || !DeviceUtils.isMobile())) {
            this.mouseHandler.enable();
        }
        
        if (this.enableTouch && DeviceUtils.hasTouch()) {
            this.touchHandler.enable();
        }
        
        if (this.enableGamepad) {
            this.gamepadHandler.enable();
        }
        
        // 启用快捷键
        this.shortcutHandler.enable();
        
        // 设置焦点处理
        this._setupFocusHandling();
        
        this.isEnabled = true;
        this.eventBus?.emit('input:enabled', { mode: this.currentMode });
    }

    /**
     * 禁用输入处理
     */
    disable() {
        if (!this.isEnabled) {
            Logger.warn('InputManager: 输入处理已禁用');
            return;
        }
        
        Logger.info('InputManager: 禁用输入处理');
        
        // 禁用所有处理器
        this.keyboardHandler.disable();
        this.mouseHandler.disable();
        this.touchHandler.disable();
        this.gamepadHandler.disable();
        this.shortcutHandler.disable();
        
        this.isEnabled = false;
        this.inputFocused = false;
        
        this.eventBus?.emit('input:disabled');
    }

    /**
     * 设置输入焦点
     */
    setFocus(focused = true) {
        this.inputFocused = focused;
        
        if (this.targetElement) {
            if (focused) {
                this.targetElement.focus();
                this.targetElement.classList.add('input-focused');
            } else {
                this.targetElement.blur();
                this.targetElement.classList.remove('input-focused');
            }
        }
        
        Logger.debug(`InputManager: 输入焦点${focused ? '已获得' : '已失去'}`);
        this.eventBus?.emit('input:focus-changed', { focused });
    }

    /**
     * 切换输入模式
     */
    switchMode(mode) {
        if (!['desktop', 'mobile', 'gamepad'].includes(mode)) {
            throw new Error(`Invalid input mode: ${mode}`);
        }
        
        const oldMode = this.currentMode;
        this.currentMode = mode;
        
        Logger.info(`InputManager: 输入模式从 ${oldMode} 切换到 ${mode}`);
        
        // 重新配置输入处理器
        if (this.isEnabled) {
            this.disable();
            this.enable();
        }
        
        this.eventBus?.emit('input:mode-changed', { oldMode, newMode: mode });
    }

    /**
     * 获取输入状态
     */
    getInputState() {
        return {
            isEnabled: this.isEnabled,
            inputFocused: this.inputFocused,
            currentMode: this.currentMode,
            enabledInputs: {
                keyboard: this.enableKeyboard && this.keyboardHandler.isEnabled,
                mouse: this.enableMouse && this.mouseHandler.isEnabled,
                touch: this.enableTouch && this.touchHandler.isEnabled,
                gamepad: this.enableGamepad && this.gamepadHandler.isEnabled
            },
            stats: { ...this.inputStats }
        };
    }

    /**
     * 获取输入统计
     */
    getInputStats() {
        return {
            ...this.inputStats,
            handlers: {
                keyboard: this.keyboardHandler.getStats(),
                mouse: this.mouseHandler.getStats(),
                touch: this.touchHandler.getStats(),
                gamepad: this.gamepadHandler.getStats()
            }
        };
    }

    /**
     * 重置输入统计
     */
    resetStats() {
        this.inputStats = {
            keyboardEvents: 0,
            mouseEvents: 0,
            touchEvents: 0,
            gamepadEvents: 0,
            totalEvents: 0,
            lastEventTime: null
        };
        
        // 重置处理器统计
        this.keyboardHandler.resetStats();
        this.mouseHandler.resetStats();
        this.touchHandler.resetStats();
        this.gamepadHandler.resetStats();
        
        Logger.info('InputManager: 输入统计已重置');
        this.eventBus?.emit('input:stats-reset');
    }

    /**
     * 检测输入模式
     * @private
     */
    _detectInputMode() {
        if (DeviceUtils.isMobile()) {
            this.currentMode = 'mobile';
        } else if (this.enableGamepad && this._hasGamepadConnected()) {
            this.currentMode = 'gamepad';
        } else {
            this.currentMode = 'desktop';
        }
        
        Logger.info(`InputManager: 检测到输入模式 - ${this.currentMode}`);
    }

    /**
     * 检查是否有游戏手柄连接
     * @private
     */
    _hasGamepadConnected() {
        if (!navigator.getGamepads) {
            return false;
        }
        
        const gamepads = navigator.getGamepads();
        for (let i = 0; i < gamepads.length; i++) {
            if (gamepads[i] && gamepads[i].connected) {
                return true;
            }
        }
        return false;
    }

    /**
     * 设置焦点处理
     * @private
     */
    _setupFocusHandling() {
        if (!this.targetElement) return;
        
        // 点击获得焦点
        this.targetElement.addEventListener('click', () => {
            this.setFocus(true);
        });
        
        // 失去焦点
        this.targetElement.addEventListener('blur', () => {
            this.setFocus(false);
        });
        
        // 容器点击也获得焦点
        if (this.videoContainer) {
            this.videoContainer.addEventListener('click', () => {
                this.setFocus(true);
            });
        }
    }
}

/**
 * KeyboardHandler - 键盘输入处理器
 */
class KeyboardHandler {
    constructor(eventBus, config) {
        this.eventBus = eventBus;
        this.config = config;
        this.isEnabled = false;
        this.targetElement = null;
        
        this.stats = {
            keydownEvents: 0,
            keyupEvents: 0,
            keypressEvents: 0
        };
        
        this.boundHandlers = {
            keydown: this._handleKeyDown.bind(this),
            keyup: this._handleKeyUp.bind(this),
            keypress: this._handleKeyPress.bind(this)
        };
    }

    setTargetElement(element) {
        this.targetElement = element;
    }

    enable() {
        if (this.isEnabled) return;
        
        document.addEventListener('keydown', this.boundHandlers.keydown);
        document.addEventListener('keyup', this.boundHandlers.keyup);
        document.addEventListener('keypress', this.boundHandlers.keypress);
        
        this.isEnabled = true;
        Logger.debug('KeyboardHandler: 已启用');
    }

    disable() {
        if (!this.isEnabled) return;
        
        document.removeEventListener('keydown', this.boundHandlers.keydown);
        document.removeEventListener('keyup', this.boundHandlers.keyup);
        document.removeEventListener('keypress', this.boundHandlers.keypress);
        
        this.isEnabled = false;
        Logger.debug('KeyboardHandler: 已禁用');
    }

    getStats() {
        return { ...this.stats };
    }

    resetStats() {
        this.stats = {
            keydownEvents: 0,
            keyupEvents: 0,
            keypressEvents: 0
        };
    }

    _handleKeyDown(event) {
        this.stats.keydownEvents++;
        this.eventBus?.emit('input:keyboard:keydown', {
            key: event.key,
            code: event.code,
            ctrlKey: event.ctrlKey,
            shiftKey: event.shiftKey,
            altKey: event.altKey,
            metaKey: event.metaKey
        });
    }

    _handleKeyUp(event) {
        this.stats.keyupEvents++;
        this.eventBus?.emit('input:keyboard:keyup', {
            key: event.key,
            code: event.code,
            ctrlKey: event.ctrlKey,
            shiftKey: event.shiftKey,
            altKey: event.altKey,
            metaKey: event.metaKey
        });
    }

    _handleKeyPress(event) {
        this.stats.keypressEvents++;
        this.eventBus?.emit('input:keyboard:keypress', {
            key: event.key,
            charCode: event.charCode
        });
    }
}

/**
 * MouseHandler - 鼠标输入处理器
 */
class MouseHandler {
    constructor(eventBus, config) {
        this.eventBus = eventBus;
        this.config = config;
        this.isEnabled = false;
        this.targetElement = null;
        
        this.sensitivity = config?.get('input.mouseSensitivity', 1.0);
        
        this.stats = {
            clickEvents: 0,
            moveEvents: 0,
            wheelEvents: 0
        };
        
        this.boundHandlers = {
            click: this._handleClick.bind(this),
            mousedown: this._handleMouseDown.bind(this),
            mouseup: this._handleMouseUp.bind(this),
            mousemove: this._handleMouseMove.bind(this),
            wheel: this._handleWheel.bind(this),
            contextmenu: this._handleContextMenu.bind(this)
        };
    }

    setTargetElement(element) {
        this.targetElement = element;
    }

    enable() {
        if (this.isEnabled || !this.targetElement) return;
        
        this.targetElement.addEventListener('click', this.boundHandlers.click);
        this.targetElement.addEventListener('mousedown', this.boundHandlers.mousedown);
        this.targetElement.addEventListener('mouseup', this.boundHandlers.mouseup);
        this.targetElement.addEventListener('mousemove', this.boundHandlers.mousemove);
        this.targetElement.addEventListener('wheel', this.boundHandlers.wheel);
        this.targetElement.addEventListener('contextmenu', this.boundHandlers.contextmenu);
        
        this.isEnabled = true;
        Logger.debug('MouseHandler: 已启用');
    }

    disable() {
        if (!this.isEnabled || !this.targetElement) return;
        
        this.targetElement.removeEventListener('click', this.boundHandlers.click);
        this.targetElement.removeEventListener('mousedown', this.boundHandlers.mousedown);
        this.targetElement.removeEventListener('mouseup', this.boundHandlers.mouseup);
        this.targetElement.removeEventListener('mousemove', this.boundHandlers.mousemove);
        this.targetElement.removeEventListener('wheel', this.boundHandlers.wheel);
        this.targetElement.removeEventListener('contextmenu', this.boundHandlers.contextmenu);
        
        this.isEnabled = false;
        Logger.debug('MouseHandler: 已禁用');
    }

    getStats() {
        return { ...this.stats };
    }

    resetStats() {
        this.stats = {
            clickEvents: 0,
            moveEvents: 0,
            wheelEvents: 0
        };
    }

    _handleClick(event) {
        this.stats.clickEvents++;
        this.eventBus?.emit('input:mouse:click', {
            button: event.button,
            x: event.offsetX,
            y: event.offsetY,
            ctrlKey: event.ctrlKey,
            shiftKey: event.shiftKey,
            altKey: event.altKey
        });
    }

    _handleMouseDown(event) {
        this.eventBus?.emit('input:mouse:mousedown', {
            button: event.button,
            x: event.offsetX,
            y: event.offsetY
        });
    }

    _handleMouseUp(event) {
        this.eventBus?.emit('input:mouse:mouseup', {
            button: event.button,
            x: event.offsetX,
            y: event.offsetY
        });
    }

    _handleMouseMove(event) {
        this.stats.moveEvents++;
        this.eventBus?.emit('input:mouse:mousemove', {
            x: event.offsetX * this.sensitivity,
            y: event.offsetY * this.sensitivity,
            movementX: event.movementX * this.sensitivity,
            movementY: event.movementY * this.sensitivity
        });
    }

    _handleWheel(event) {
        event.preventDefault();
        this.stats.wheelEvents++;
        this.eventBus?.emit('input:mouse:wheel', {
            deltaX: event.deltaX,
            deltaY: event.deltaY,
            deltaZ: event.deltaZ,
            deltaMode: event.deltaMode
        });
    }

    _handleContextMenu(event) {
        event.preventDefault();
        this.eventBus?.emit('input:mouse:contextmenu', {
            x: event.offsetX,
            y: event.offsetY
        });
    }
}

/**
 * TouchHandler - 触摸输入处理器
 */
class TouchHandler {
    constructor(eventBus, config) {
        this.eventBus = eventBus;
        this.config = config;
        this.isEnabled = false;
        this.targetElement = null;
        
        this.sensitivity = config?.get('input.mobile.touchSensitivity', 1.0);
        this.preventZoom = config?.get('input.mobile.preventZoom', true);
        
        this.stats = {
            touchStartEvents: 0,
            touchMoveEvents: 0,
            touchEndEvents: 0,
            gestureEvents: 0
        };
        
        this.boundHandlers = {
            touchstart: this._handleTouchStart.bind(this),
            touchmove: this._handleTouchMove.bind(this),
            touchend: this._handleTouchEnd.bind(this),
            touchcancel: this._handleTouchCancel.bind(this)
        };
        
        // 手势识别
        this.gestureRecognizer = new GestureRecognizer(eventBus);
    }

    setTargetElement(element) {
        this.targetElement = element;
    }

    enable() {
        if (this.isEnabled || !this.targetElement) return;
        
        this.targetElement.addEventListener('touchstart', this.boundHandlers.touchstart, { passive: false });
        this.targetElement.addEventListener('touchmove', this.boundHandlers.touchmove, { passive: false });
        this.targetElement.addEventListener('touchend', this.boundHandlers.touchend, { passive: false });
        this.targetElement.addEventListener('touchcancel', this.boundHandlers.touchcancel, { passive: false });
        
        // 防止缩放
        if (this.preventZoom) {
            this._preventZoom();
        }
        
        this.isEnabled = true;
        Logger.debug('TouchHandler: 已启用');
    }

    disable() {
        if (!this.isEnabled || !this.targetElement) return;
        
        this.targetElement.removeEventListener('touchstart', this.boundHandlers.touchstart);
        this.targetElement.removeEventListener('touchmove', this.boundHandlers.touchmove);
        this.targetElement.removeEventListener('touchend', this.boundHandlers.touchend);
        this.targetElement.removeEventListener('touchcancel', this.boundHandlers.touchcancel);
        
        this.isEnabled = false;
        Logger.debug('TouchHandler: 已禁用');
    }

    getStats() {
        return { ...this.stats };
    }

    resetStats() {
        this.stats = {
            touchStartEvents: 0,
            touchMoveEvents: 0,
            touchEndEvents: 0,
            gestureEvents: 0
        };
    }

    _handleTouchStart(event) {
        if (this.preventZoom && event.touches.length > 1) {
            event.preventDefault();
        }
        
        this.stats.touchStartEvents++;
        const touches = this._normalizeTouches(event.touches);
        
        this.gestureRecognizer.handleTouchStart(touches);
        this.eventBus?.emit('input:touch:touchstart', { touches });
    }

    _handleTouchMove(event) {
        event.preventDefault();
        
        this.stats.touchMoveEvents++;
        const touches = this._normalizeTouches(event.touches);
        
        this.gestureRecognizer.handleTouchMove(touches);
        this.eventBus?.emit('input:touch:touchmove', { touches });
    }

    _handleTouchEnd(event) {
        this.stats.touchEndEvents++;
        const touches = this._normalizeTouches(event.changedTouches);
        
        this.gestureRecognizer.handleTouchEnd(touches);
        this.eventBus?.emit('input:touch:touchend', { touches });
    }

    _handleTouchCancel(event) {
        const touches = this._normalizeTouches(event.changedTouches);
        
        this.gestureRecognizer.handleTouchCancel(touches);
        this.eventBus?.emit('input:touch:touchcancel', { touches });
    }

    _normalizeTouches(touches) {
        const rect = this.targetElement.getBoundingClientRect();
        return Array.from(touches).map(touch => ({
            id: touch.identifier,
            x: (touch.clientX - rect.left) * this.sensitivity,
            y: (touch.clientY - rect.top) * this.sensitivity,
            force: touch.force || 1.0
        }));
    }

    _preventZoom() {
        // 防止双击缩放
        let lastTouchEnd = 0;
        document.addEventListener('touchend', (event) => {
            const now = Date.now();
            if (now - lastTouchEnd <= 300) {
                event.preventDefault();
            }
            lastTouchEnd = now;
        }, false);
        
        // 防止手势缩放
        document.addEventListener('gesturestart', (event) => {
            event.preventDefault();
        });
        
        document.addEventListener('gesturechange', (event) => {
            event.preventDefault();
        });
        
        document.addEventListener('gestureend', (event) => {
            event.preventDefault();
        });
    }
}

/**
 * GamepadHandler - 游戏手柄输入处理器
 */
class GamepadHandler {
    constructor(eventBus, config) {
        this.eventBus = eventBus;
        this.config = config;
        this.isEnabled = false;
        
        this.stats = {
            buttonEvents: 0,
            axisEvents: 0
        };
        
        this.gamepads = {};
        this.pollTimer = null;
        this.pollInterval = 16; // ~60fps
    }

    enable() {
        if (this.isEnabled) return;
        
        window.addEventListener('gamepadconnected', this._handleGamepadConnected.bind(this));
        window.addEventListener('gamepaddisconnected', this._handleGamepadDisconnected.bind(this));
        
        this._startPolling();
        
        this.isEnabled = true;
        Logger.debug('GamepadHandler: 已启用');
    }

    disable() {
        if (!this.isEnabled) return;
        
        window.removeEventListener('gamepadconnected', this._handleGamepadConnected.bind(this));
        window.removeEventListener('gamepaddisconnected', this._handleGamepadDisconnected.bind(this));
        
        this._stopPolling();
        
        this.isEnabled = false;
        Logger.debug('GamepadHandler: 已禁用');
    }

    getStats() {
        return { ...this.stats };
    }

    resetStats() {
        this.stats = {
            buttonEvents: 0,
            axisEvents: 0
        };
    }

    _handleGamepadConnected(event) {
        Logger.info(`GamepadHandler: 游戏手柄已连接 - ${event.gamepad.id}`);
        this.gamepads[event.gamepad.index] = event.gamepad;
        this.eventBus?.emit('input:gamepad:connected', { gamepad: event.gamepad });
    }

    _handleGamepadDisconnected(event) {
        Logger.info(`GamepadHandler: 游戏手柄已断开 - ${event.gamepad.id}`);
        delete this.gamepads[event.gamepad.index];
        this.eventBus?.emit('input:gamepad:disconnected', { gamepad: event.gamepad });
    }

    _startPolling() {
        this.pollTimer = setInterval(() => {
            this._pollGamepads();
        }, this.pollInterval);
    }

    _stopPolling() {
        if (this.pollTimer) {
            clearInterval(this.pollTimer);
            this.pollTimer = null;
        }
    }

    _pollGamepads() {
        const gamepads = navigator.getGamepads();
        
        for (let i = 0; i < gamepads.length; i++) {
            const gamepad = gamepads[i];
            if (gamepad && gamepad.connected) {
                this._processGamepad(gamepad);
            }
        }
    }

    _processGamepad(gamepad) {
        // 处理按钮
        gamepad.buttons.forEach((button, index) => {
            if (button.pressed) {
                this.stats.buttonEvents++;
                this.eventBus?.emit('input:gamepad:button', {
                    gamepadIndex: gamepad.index,
                    buttonIndex: index,
                    pressed: button.pressed,
                    value: button.value
                });
            }
        });
        
        // 处理摇杆
        gamepad.axes.forEach((axis, index) => {
            if (Math.abs(axis) > 0.1) { // 死区
                this.stats.axisEvents++;
                this.eventBus?.emit('input:gamepad:axis', {
                    gamepadIndex: gamepad.index,
                    axisIndex: index,
                    value: axis
                });
            }
        });
    }
}

/**
 * ShortcutHandler - 快捷键处理器
 */
class ShortcutHandler {
    constructor(eventBus, config) {
        this.eventBus = eventBus;
        this.config = config;
        this.isEnabled = false;
        this.targetElement = null;
        
        this.shortcuts = new Map();
        this._loadDefaultShortcuts();
        
        this.boundHandler = this._handleKeyDown.bind(this);
    }

    setTargetElement(element) {
        this.targetElement = element;
    }

    enable() {
        if (this.isEnabled) return;
        
        document.addEventListener('keydown', this.boundHandler);
        
        this.isEnabled = true;
        Logger.debug('ShortcutHandler: 已启用');
    }

    disable() {
        if (!this.isEnabled) return;
        
        document.removeEventListener('keydown', this.boundHandler);
        
        this.isEnabled = false;
        Logger.debug('ShortcutHandler: 已禁用');
    }

    registerShortcut(key, callback, description = '') {
        this.shortcuts.set(key.toLowerCase(), { callback, description });
        Logger.debug(`ShortcutHandler: 注册快捷键 ${key}`);
    }

    unregisterShortcut(key) {
        this.shortcuts.delete(key.toLowerCase());
        Logger.debug(`ShortcutHandler: 取消注册快捷键 ${key}`);
    }

    getShortcuts() {
        const shortcuts = {};
        this.shortcuts.forEach((value, key) => {
            shortcuts[key] = value.description;
        });
        return shortcuts;
    }

    _loadDefaultShortcuts() {
        const shortcuts = this.config?.get('input.shortcuts', {});
        
        // 默认快捷键
        const defaultShortcuts = {
            'F11': () => this._toggleFullscreen(),
            'Ctrl+Shift+C': () => this.eventBus?.emit('ui:toggle-controls'),
            'Ctrl+Shift+S': () => this.eventBus?.emit('ui:toggle-stats'),
            'Ctrl+Shift+P': () => this.eventBus?.emit('ui:screenshot'),
            'Escape': () => this._handleEscape(),
            ' ': () => this._handleSpace(),
            'm': () => this._handleMute()
        };
        
        // 合并配置的快捷键
        const allShortcuts = { ...defaultShortcuts, ...shortcuts };
        
        Object.entries(allShortcuts).forEach(([key, action]) => {
            if (typeof action === 'function') {
                this.shortcuts.set(key.toLowerCase(), { callback: action, description: '' });
            }
        });
    }

    _handleKeyDown(event) {
        const key = this._getKeyString(event);
        const shortcut = this.shortcuts.get(key.toLowerCase());
        
        if (shortcut) {
            event.preventDefault();
            try {
                shortcut.callback(event);
                Logger.debug(`ShortcutHandler: 执行快捷键 ${key}`);
            } catch (error) {
                Logger.error(`ShortcutHandler: 快捷键执行失败 ${key}`, error);
            }
        }
    }

    _getKeyString(event) {
        const parts = [];
        
        if (event.ctrlKey) parts.push('Ctrl');
        if (event.shiftKey) parts.push('Shift');
        if (event.altKey) parts.push('Alt');
        if (event.metaKey) parts.push('Meta');
        
        parts.push(event.key);
        
        return parts.join('+');
    }

    _toggleFullscreen() {
        if (document.fullscreenElement) {
            document.exitFullscreen();
        } else {
            document.documentElement.requestFullscreen();
        }
    }

    _handleEscape() {
        if (document.fullscreenElement) {
            document.exitFullscreen();
        }
    }

    _handleSpace() {
        // 只有在输入焦点时才处理空格键
        if (this.targetElement && document.activeElement === this.targetElement) {
            this.eventBus?.emit('ui:toggle-play');
        }
    }

    _handleMute() {
        // 只有在输入焦点时才处理静音键
        if (this.targetElement && document.activeElement === this.targetElement) {
            this.eventBus?.emit('ui:toggle-mute');
        }
    }
}

/**
 * GestureRecognizer - 手势识别器
 */
class GestureRecognizer {
    constructor(eventBus) {
        this.eventBus = eventBus;
        this.touches = new Map();
        this.gestures = [];
        
        // 手势参数
        this.tapTimeout = 300;
        this.tapThreshold = 10;
        this.swipeThreshold = 50;
        this.pinchThreshold = 10;
    }

    handleTouchStart(touches) {
        touches.forEach(touch => {
            this.touches.set(touch.id, {
                startX: touch.x,
                startY: touch.y,
                currentX: touch.x,
                currentY: touch.y,
                startTime: Date.now()
            });
        });
    }

    handleTouchMove(touches) {
        touches.forEach(touch => {
            const touchData = this.touches.get(touch.id);
            if (touchData) {
                touchData.currentX = touch.x;
                touchData.currentY = touch.y;
            }
        });
        
        // 检测多点触控手势
        if (touches.length === 2) {
            this._detectPinch(touches);
        }
    }

    handleTouchEnd(touches) {
        touches.forEach(touch => {
            const touchData = this.touches.get(touch.id);
            if (touchData) {
                this._detectGesture(touch, touchData);
                this.touches.delete(touch.id);
            }
        });
    }

    handleTouchCancel(touches) {
        touches.forEach(touch => {
            this.touches.delete(touch.id);
        });
    }

    _detectGesture(touch, touchData) {
        const deltaX = touch.x - touchData.startX;
        const deltaY = touch.y - touchData.startY;
        const distance = Math.sqrt(deltaX * deltaX + deltaY * deltaY);
        const duration = Date.now() - touchData.startTime;
        
        if (distance < this.tapThreshold && duration < this.tapTimeout) {
            // 点击手势
            this.eventBus?.emit('input:gesture:tap', {
                x: touch.x,
                y: touch.y,
                duration
            });
        } else if (distance > this.swipeThreshold) {
            // 滑动手势
            const direction = this._getSwipeDirection(deltaX, deltaY);
            this.eventBus?.emit('input:gesture:swipe', {
                direction,
                distance,
                deltaX,
                deltaY,
                duration
            });
        }
    }

    _detectPinch(touches) {
        if (touches.length !== 2) return;
        
        const touch1 = touches[0];
        const touch2 = touches[1];
        
        const distance = Math.sqrt(
            Math.pow(touch2.x - touch1.x, 2) + Math.pow(touch2.y - touch1.y, 2)
        );
        
        // 这里可以实现缩放手势检测
        this.eventBus?.emit('input:gesture:pinch', {
            distance,
            centerX: (touch1.x + touch2.x) / 2,
            centerY: (touch1.y + touch2.y) / 2
        });
    }

    _getSwipeDirection(deltaX, deltaY) {
        const absX = Math.abs(deltaX);
        const absY = Math.abs(deltaY);
        
        if (absX > absY) {
            return deltaX > 0 ? 'right' : 'left';
        } else {
            return deltaY > 0 ? 'down' : 'up';
        }
    }
}

// 导出类
if (typeof module !== 'undefined' && module.exports) {
    module.exports = {
        InputManager,
        KeyboardHandler,
        MouseHandler,
        TouchHandler,
        GamepadHandler,
        ShortcutHandler,
        GestureRecognizer
    };
} else if (typeof window !== 'undefined') {
    window.InputManager = InputManager;
    window.KeyboardHandler = KeyboardHandler;
    window.MouseHandler = MouseHandler;
    window.TouchHandler = TouchHandler;
    window.GamepadHandler = GamepadHandler;
    window.ShortcutHandler = ShortcutHandler;
    window.GestureRecognizer = GestureRecognizer;
}