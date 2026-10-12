/*
 * Copyright 2026 Glenn Lewis. All rights reserved.
 *
 * This program is free software: you can redistribute it and/or modify
 * it under the terms of the GNU General Public License as published by
 * the Free Software Foundation, either version 3 of the License, or
 * (at your option) any later version.
 *
 * This program is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
 * GNU General Public License for more details.
 *
 * You should have received a copy of the GNU General Public License
 * along with this program. If not, see <https://www.gnu.org/licenses/>.
 */

/*
 * bridge.c is what makes bridge.h a module Swift can import.
 *
 * Every function the header declares is implemented by the static archive the build
 * script produces from ./cmd/gonomadnet, so this file defines none of them: it exists
 * so that the header is compiled once, on its own, where the compiler can say so if a
 * declaration cannot work. A bridging header that is only ever pasted into another
 * target's build settings is the kind of file that goes wrong quietly.
 */

#include "bridge.h"
